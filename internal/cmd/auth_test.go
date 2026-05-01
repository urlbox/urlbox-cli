package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
)

func TestAuth_RequiresAPISecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on missing --api-secret")
	}
}

func TestAuth_WritesConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_xxxxxxxxxxxx"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	if got := config.ResolveAPISecret(); got != "sec_xxxxxxxxxxxx" {
		t.Fatalf("secret not persisted; got %q", got)
	}
}

func TestAuth_FlagPath_StoresInAPISecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_xxx"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Profiles["default"].APISecret != "sec_xxx" {
		t.Errorf("APISecret = %q, want sec_xxx", c.Profiles["default"].APISecret)
	}
	if c.Profiles["default"].APIKey != "" {
		t.Errorf("APIKey unexpectedly populated: %q (publishable-key field; should be empty after auth)", c.Profiles["default"].APIKey)
	}
}

func TestAuth_OldAPIKeyFlag_RemovedFromHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"auth", "--help"}, &stdout, &stderr)
	help := stdout.String() + stderr.String()
	if strings.Contains(help, "--api-key") {
		t.Error("--api-key should be gone in v0.6.0; replaced by --api-secret")
	}
	if !strings.Contains(help, "--api-secret") {
		t.Error("--api-secret missing from --help")
	}
}

// Regression guard: --help and the missing-secret error envelope must both
// point users at the dashboard URL where they can copy their API secret.
// Field-report observation: agents (and humans) were inventing wrong URLs
// (urlbox.com/dashboard/api-secrets, etc.) because the CLI never said where
// to find the secret. Pinning the canonical pointer here.
func TestAuth_HelpAndErrorPointAtDashboardURL(t *testing.T) {
	const wantURL = "urlbox.com/dashboard/projects"

	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"auth", "--help"}, &stdout, &stderr)
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, wantURL) {
		t.Errorf("--help should point at %q so users know where to grab their secret; got:\n%s", wantURL, help)
	}

	// Now exercise the missing-secret path and check the error envelope's hint.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on missing --api-secret")
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout: %s", err, stdout.String())
	}
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, wantURL) {
		t.Errorf("missing-secret hint should point at %q; got %q", wantURL, hint)
	}
}

func TestAuth_OutputEnvelopeMasksSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_supersecretvalue", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nout: %s", err, stdout.String())
	}
	summary, _ := env["summary"].(string)
	if strings.Contains(summary, "supersecretvalue") {
		t.Fatalf("summary should mask the secret: %q", summary)
	}
	if !strings.Contains(summary, "sec_") {
		t.Fatalf("summary should show prefix: %q", summary)
	}
	data, _ := env["data"].(map[string]any)
	if ms, _ := data["masked_secret"].(string); strings.Contains(ms, "supersecretvalue") {
		t.Fatalf("masked_secret leaked secret: %q", ms)
	}
}

func TestAuth_RejectsEmptySecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", ""}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on empty secret")
	}
}

func TestAuth_HasBreadcrumbs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"auth", "--api-secret", "sec_test1234", "--output-format", "json"}, &stdout, &stderr)

	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Fatalf("expected breadcrumbs, got: %v", env["breadcrumbs"])
	}
}

func TestAuth_NonInteractive_NoSecret_StillUsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd.SetStdinTTYForTest(false)
	defer cmd.ResetStdinTTYForTest()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage)", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v", env["code"])
	}
	if env["error"] != "missing --api-secret" {
		t.Errorf("error=%v", env["error"])
	}
}

// The interactive path requires a pty — we don't drive a real pty in CI.
// Instead we inject a stub secret-reader and verify the dispatcher selects
// the interactive branch when both stdin and stderr are TTYs.
func TestAuth_InteractivePath_DispatchedWhenTTY(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd.SetStdinTTYForTest(true)
	cmd.SetStderrTTYForTest(true)
	defer cmd.ResetStdinTTYForTest()
	defer cmd.ResetStderrTTYForTest()

	called := false
	cmd.SetAuthSecretReaderForTest(func() (string, error) {
		called = true
		return "sec_prompt", nil
	})
	defer cmd.ResetAuthSecretReaderForTest()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if !called {
		t.Fatal("interactive secret reader was not called")
	}
	if !strings.Contains(stderr.String(), "API secret:") {
		t.Errorf("expected prompt label on stderr, got %q", stderr.String())
	}

	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Profiles["default"].APISecret != "sec_prompt" {
		t.Errorf("APISecret = %q, want sec_prompt", c.Profiles["default"].APISecret)
	}
}

func TestAuth_InteractivePath_EmptyInput_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd.SetStdinTTYForTest(true)
	cmd.SetStderrTTYForTest(true)
	defer cmd.ResetStdinTTYForTest()
	defer cmd.ResetStderrTTYForTest()

	cmd.SetAuthSecretReaderForTest(func() (string, error) {
		return "", nil
	})
	defer cmd.ResetAuthSecretReaderForTest()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v", env["code"])
	}
	// The interactive empty-input error must NOT suggest "run interactively
	// on a TTY" — the user just did that. It should say so.
	if env["error"] != "empty API secret" {
		t.Errorf("error=%v, want %q", env["error"], "empty API secret")
	}
	if hint, _ := env["hint"].(string); strings.Contains(hint, "run interactively on a TTY") {
		t.Errorf("interactive hint shouldn't suggest TTY (user is already on one): %q", hint)
	}
}
