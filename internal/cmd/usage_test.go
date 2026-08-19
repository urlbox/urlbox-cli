package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestUsageHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"rendersUsed":120,"renderQuota":1000,"period":{"start":"2026-08-01","end":"2026-08-31"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"usage", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	var env struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Data["renders_used"] != float64(120) || env.Data["render_quota"] != float64(1000) {
		t.Fatalf("data: %#v", env.Data)
	}
	if env.Data["current_period_start"] != "2026-08-01" {
		t.Fatalf("period: %#v", env.Data)
	}
}

func TestUsageNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"usage", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
}
