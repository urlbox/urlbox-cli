//go:build smoke

// Smoke tests exercise internal/api against the real api.urlbox.com.
// They are isolated behind the `smoke` build tag so `make ci` /
// `go test ./...` never trigger them. Run with:
//
//	URLBOX_API_SECRET=ubx_sk_... make smoke
//
// Each render burns one credit on the configured account. Keep this
// suite small and decisive — it's a release-cut ritual, not a
// continuous feedback loop.
//
// All tests skip cleanly when URLBOX_API_SECRET is unset, so the
// `smoke` build tag is safe to enable in any environment without
// silently hitting the API.
package api_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// requireSecret returns the configured API secret or skips the test.
// All smoke tests start with this guard.
func requireSecret(t *testing.T) string {
	t.Helper()
	s := os.Getenv(config.EnvAPISecret)
	if s == "" {
		t.Skip("URLBOX_API_SECRET not set; skipping smoke test")
	}
	return s
}

func smokeClient(t *testing.T) *api.HTTPClient {
	t.Helper()
	c := api.NewHTTPClient(api.ResolveAPIHost(), "", requireSecret(t))
	// Smoke runs are deliberate; don't burn credits on retry storms.
	c.Retry = api.NoRetryConfig()
	return c
}

func TestSmoke_RenderSync(t *testing.T) {
	c := smokeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := c.Render(ctx, map[string]any{
		"url":    "https://example.com",
		"format": "png",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Locked spec: sync response carries renderUrl, size, renderTime,
	// queueTime, width, height (camelCase across the wire).
	for _, key := range []string{"renderUrl", "size", "renderTime", "width", "height"} {
		if resp.Data[key] == nil {
			t.Errorf("response missing %q (got keys: %v)", key, mapKeys(resp.Data))
		}
	}
	url, _ := resp.Data["renderUrl"].(string)
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("renderUrl=%q, want https:// prefix", url)
	}
	size, _ := resp.Data["size"].(float64)
	if size < 100 {
		t.Errorf("size=%v, suspiciously small for a real render", size)
	}
}

func TestSmoke_RenderAsync_QueuesImmediately(t *testing.T) {
	c := smokeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Async returns 201 immediately with status=created + renderId; we
	// don't poll the resulting render here (Phase 5 will). One credit
	// is deducted whenever the queued render later completes.
	resp, err := c.RenderAsync(ctx, map[string]any{
		"url":    "https://example.com",
		"format": "png",
	})
	if err != nil {
		t.Fatalf("RenderAsync: %v", err)
	}
	for _, key := range []string{"status", "renderId", "statusUrl"} {
		if resp.Data[key] == nil {
			t.Errorf("response missing %q (got keys: %v)", key, mapKeys(resp.Data))
		}
	}
	if status, _ := resp.Data["status"].(string); status != "created" {
		t.Errorf("status=%q, want %q", status, "created")
	}
	id, _ := resp.Data["renderId"].(string)
	if id == "" {
		t.Errorf("renderId is empty")
	}
}

// TestSmoke_AuthFails_BadSecret confirms a bogus secret produces ErrAuth
// regardless of HTTP status. The real Urlbox API returns 400 with a nested
// {"error":{"code":"ApiKeyNotFound"}} body — not 401 — so the HTTPClient's
// auth mapping must inspect the error code, not just the status code.
//
// This test deliberately uses a bogus secret so it doesn't burn credits;
// we still gate on URLBOX_API_SECRET being set so the smoke suite runs
// predictably (all-or-nothing).
func TestSmoke_AuthFails_BadSecret(t *testing.T) {
	requireSecret(t)
	c := api.NewHTTPClient(api.ResolveAPIHost(), "", "ubx_sk_definitely_not_a_real_secret")
	c.Retry = api.NoRetryConfig()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.Render(ctx, map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("err=%v, want *output.CLIError", err)
	}
	if cli.Code != output.ErrAuth {
		t.Errorf("Code=%q, want %q (bad secret should map to auth regardless of HTTP status)", cli.Code, output.ErrAuth)
	}
	if !strings.Contains(cli.Hint, "urlbox auth") {
		t.Errorf("Hint=%q, want pointer to `urlbox auth`", cli.Hint)
	}
}

// mapKeys is a debug helper for test failure messages.
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
