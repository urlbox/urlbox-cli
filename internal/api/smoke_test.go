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
	if !strings.Contains(cli.Hint, "urlbox login") {
		t.Errorf("Hint=%q, want pointer to `urlbox login`", cli.Hint)
	}
}

// TestSmoke_v090_PassthroughVideoScroll proves the v0.9.0 schema-as-docs
// contract end-to-end: video_scroll fields (no longer in schema/render.json
// after the v0.9.0 hand-patch removal) flow through to the API and produce
// a real video render. Pre-v0.9.0 the CLI rejected this locally with an
// "unknown option" envelope; today it's just an option the API knows about.
func TestSmoke_v090_PassthroughVideoScroll(t *testing.T) {
	c := smokeClient(t)
	// Video renders take longer than screenshots; give the per-attempt
	// timeout some headroom.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	resp, err := c.Render(ctx, map[string]any{
		"url":                   "https://example.com",
		"format":                "mp4",
		"video_scroll":          true,
		"video_scroll_distance": 600,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	url, _ := resp.Data["renderUrl"].(string)
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("renderUrl=%q, want https:// prefix", url)
	}
}

// TestSmoke_v090_PassthroughTotallyMadeUp proves the loose-schema contract:
// fields the API doesn't know about are silently accepted (engine ignores).
// This is the "agent typed something completely novel" case — the CLI does
// not gate, so it must not break the request when the API also doesn't gate.
func TestSmoke_v090_PassthroughTotallyMadeUp(t *testing.T) {
	c := smokeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := c.Render(ctx, map[string]any{
		"url":                       "https://example.com",
		"format":                    "png",
		"totally_made_up_xyz_2026":  true,
		"and_another_one_for_color": "purple",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	url, _ := resp.Data["renderUrl"].(string)
	if !strings.HasPrefix(url, "https://") {
		t.Errorf("renderUrl=%q, want https:// prefix", url)
	}
}

// TestSmoke_v090_KnownBadType_APIReturnsMeaningfulError confirms the OTHER
// half of the v0.9.0 contract: when --json carries a known field with a
// bad type, the API rejects with a structured response that the CLI maps
// to ErrValidation (NOT the generic ErrUsage default that pre-v0.9.0 used
// for "other 4xx"). Sends width as an array (no anyOf branch matches: not
// a string, not an integer) to guarantee a hard rejection.
//
// What we can verify on the wire today:
//   - error code routes to ErrValidation (the routing fix)
//   - message is non-empty and signals options-level rejection
//   - hint is non-empty and points the agent at next steps
//
// What we CAN'T verify here (deferred to a follow-up urlbox-mono PR):
// the API's response body does not include `info.errors` (the Zod tree
// with field names) — only the generic "Invalid options, please check
// errors" message + the `InvalidOptions` code. Field-level detail would
// require an API change to add `info.errors` to the wire response.
func TestSmoke_v090_KnownBadType_APIReturnsMeaningfulError(t *testing.T) {
	c := smokeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := c.Render(ctx, map[string]any{
		"url":   "https://example.com",
		"width": []any{1, 2, 3},
	})
	if err == nil {
		t.Fatalf("expected API to reject width:[1,2,3]; got success")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("err=%v, want *output.CLIError", err)
	}
	if cli.Code != output.ErrValidation {
		t.Errorf("Code=%q, want %q (API option-validation should map to validation, not usage)", cli.Code, output.ErrValidation)
	}
	if cli.Message == "" {
		t.Errorf("Message is empty; want a non-empty rejection message from the API")
	}
	if cli.Hint == "" {
		t.Errorf("Hint is empty; want a pointer to next-step (e.g. urlbox schema render)")
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
