package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
	if env["error"] != "missing API secret" {
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

// Regression guard: the trailing newline emitted after the masked password
// read must land on the cobra-injected stderr writer (cmd.ErrOrStderr()),
// not on the process-global os.Stderr. Otherwise tests can't capture or
// redirect it, and the writer-plumbing convention breaks.
func TestAuth_InteractivePrompt_NewlineGoesToCobraStderr(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd.SetStdinTTYForTest(true)
	cmd.SetStderrTTYForTest(true)
	t.Cleanup(func() {
		cmd.ResetStdinTTYForTest()
		cmd.ResetStderrTTYForTest()
	})
	cmd.SetAuthSecretReaderForTest(func() (string, error) {
		return "ubx_sk_test12345678", nil
	})
	t.Cleanup(cmd.ResetAuthSecretReaderForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "API secret:") {
		t.Errorf("stderr missing prompt; got %q", stderr.String())
	}
	if !strings.HasSuffix(stderr.String(), "\n") {
		t.Errorf("stderr missing trailing newline; got %q", stderr.String())
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

// TestAuth_APISecretStdin_ReadsAndSaves pins the --api-secret-stdin path:
// pipe the secret on stdin, no argv leak, no shell-history exposure.
// Closes Round 1 S-C2.
func TestAuth_APISecretStdin_ReadsAndSaves(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetSecretStdinForTest(strings.NewReader("sec_stdin_abcdefghij\n"))
	t.Cleanup(cmd.ResetSecretStdinForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret-stdin"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Profiles[c.DefaultProfile].APISecret; got != "sec_stdin_abcdefghij" {
		t.Errorf("APISecret=%q, want sec_stdin_abcdefghij", got)
	}
	// stdin path must NOT trigger the TTY history warning.
	if strings.Contains(stderr.String(), "shell history") {
		t.Errorf("stdin path should not warn about shell history; got %q", stderr.String())
	}
}

// TestAuth_APISecretFile_ReadsAndSaves pins the --api-secret-file path.
func TestAuth_APISecretFile_ReadsAndSaves(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	dir := t.TempDir()
	p := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(p, []byte("sec_file_klmnopqrst\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret-file", p}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Profiles[c.DefaultProfile].APISecret; got != "sec_file_klmnopqrst" {
		t.Errorf("APISecret=%q, want sec_file_klmnopqrst", got)
	}
}

// TestAuth_APISecretFlag_OnTTY_PrintsHistoryWarning pins UX I5:
// --api-secret <value> on a TTY emits a stderr warning about shell history.
// On a non-TTY (CI), the warning is suppressed.
func TestAuth_APISecretFlag_OnTTY_PrintsHistoryWarning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStderrTTYForTest(true)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_argv_uvwxyz1234"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "shell history") {
		t.Errorf("TTY stderr should warn about shell history; got %q", stderr.String())
	}
}

// TestAuth_APISecretFlag_OnNonTTY_NoWarning pins suppression in CI / pipelines.
func TestAuth_APISecretFlag_OnNonTTY_NoWarning(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_ci_qwerty5678"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "warning") {
		t.Errorf("non-TTY should NOT emit shell-history warning; got %q", stderr.String())
	}
}

// TestAuth_MutexBetweenSecretInputFlags pins that passing more than one of
// --api-secret, --api-secret-stdin, --api-secret-file is a usage error.
func TestAuth_MutexBetweenSecretInputFlags(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_x", "--api-secret-stdin"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on mutex violation")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	if !strings.Contains(env["error"].(string), "at most one") {
		t.Errorf("error should say 'at most one'; got %q", env["error"])
	}
}
