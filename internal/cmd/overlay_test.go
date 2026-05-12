package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

// TestOverlay_RenderDryRun_PicksUpOverlay pins Round 8 HH: before this
// commit, `.urlbox/config.json` was advertised but never read. With
// overlay loading wired in, the api_host from the overlay should
// surface through the resolver (verified via `urlbox link` whose
// --curl prints the resolved URL — but a simpler proof is doctor JSON
// since it reports overlay-supplied values too. Easiest test: ensure
// the overlay is honored when no other source supplies api_host).
func TestOverlay_RenderDryRun_PicksUpAPIKey(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("URLBOX_API_SECRET", "")
	t.Setenv("URLBOX_API_HOST", "")

	// Working dir with a `.urlbox/config.json` overlay providing both
	// api_key and api_secret. Set HOME to a temp dir so the walk stops.
	workdir := filepath.Join(xdg, "myproj")
	if err := os.MkdirAll(filepath.Join(workdir, ".urlbox"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	overlay := map[string]string{
		"api_key":    "pub_overlay_key_aaa",
		"api_secret": "sec_overlay_secret_bbb",
	}
	b, _ := json.Marshal(overlay)
	if err := os.WriteFile(filepath.Join(workdir, ".urlbox", "config.json"), b, 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	prevWd, _ := os.Getwd()
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("render --dry-run with overlay should succeed; exit=%d stderr=%s", exit, stderr.String())
	}
	// The overlay's secret must NOT leak into the dry-run echo (only the
	// rendered payload should). But the fact that we got exit 0 means
	// the resolver found a secret somewhere — overlay was the only source.
	if strings.Contains(stdout.String(), "sec_overlay_secret_bbb") {
		t.Errorf("overlay secret leaked into render dry-run envelope: %s", stdout.String())
	}
}

// TestOverlay_MalformedJSON_ProducesCleanError pins that broken overlay
// files surface a usage error naming the file (so the user knows what
// to fix), not a silent fallback.
func TestOverlay_MalformedJSON_ProducesCleanError(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("URLBOX_API_SECRET", "")

	workdir := filepath.Join(xdg, "myproj")
	if err := os.MkdirAll(filepath.Join(workdir, ".urlbox"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workdir, ".urlbox", "config.json"), []byte("NOT JSON {["), 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	prevWd, _ := os.Getwd()
	if err := os.Chdir(workdir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWd) })

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("malformed overlay should error; got exit 0 stdout=%s", stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	if errMsg, ok := env["error"].(string); !ok || !strings.Contains(errMsg, "config.json") {
		t.Errorf("error should name the malformed file; got %q", env["error"])
	}
}
