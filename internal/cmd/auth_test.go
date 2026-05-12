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

// TestAuth_Overwrite_NonTTY_RequiresForce pins S-C3 non-TTY behavior:
// when the default profile already has a secret AND a different new
// secret arrives non-interactively, refuse to overwrite. This is the
// 2026-05-08 incident-class guard. Pass --force to opt in.
func TestAuth_Overwrite_NonTTY_RequiresForce(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	// Seed an existing secret.
	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_original_abcdef"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed exit=%d", exit)
	}

	// Attempt a different secret WITHOUT --force.
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_DIFFERENT_uvwxyz", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit when overwrite refused in non-TTY mode")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "conflict" {
		t.Errorf("code=%v, want conflict", env["code"])
	}
	if hint, _ := env["hint"].(string); !strings.Contains(hint, "--force") {
		t.Errorf("hint should mention --force; got %q", hint)
	}

	// Verify original secret was NOT clobbered.
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Profiles[c.DefaultProfile].APISecret != "sec_original_abcdef" {
		t.Errorf("secret was clobbered; got %q, want sec_original_abcdef",
			c.Profiles[c.DefaultProfile].APISecret)
	}
}

// TestAuth_Overwrite_Force_Overwrites pins that --force bypasses the
// guard non-interactively.
func TestAuth_Overwrite_Force_Overwrites(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_original_abcdef"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed exit=%d", exit)
	}
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_NEW_overwritten12", "--force"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d (--force should permit overwrite); stderr=%s", exit, stderr.String())
	}
	c, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Profiles[c.DefaultProfile].APISecret != "sec_NEW_overwritten12" {
		t.Errorf("--force did not overwrite; got %q", c.Profiles[c.DefaultProfile].APISecret)
	}
}

// TestAuth_Overwrite_SameSecret_NoGuard pins the idempotent case: if the
// new secret matches the existing, no guard / prompt fires.
func TestAuth_Overwrite_SameSecret_NoGuard(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_identical_xyz789"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed exit=%d", exit)
	}
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_identical_xyz789"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d (same-secret re-save should succeed silently)", exit)
	}
}

// TestAuth_Overwrite_TTY_Prompts pins S-C3 TTY behavior: prompt y/N
// via the test-injectable confirm-reader. "y" accepts, "n" cancels.
func TestAuth_Overwrite_TTY_PromptAccept(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(true)
	cmd.SetStderrTTYForTest(true)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	cmd.SetStdinTTYForTest(false)
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_seed_abcdefghij"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed exit=%d", exit)
	}
	cmd.SetStdinTTYForTest(true)

	cmd.SetAuthConfirmReaderForTest(func() (string, error) { return "y", nil })
	t.Cleanup(cmd.ResetAuthConfirmReaderForTest)

	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_replaced_xyz123"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d, want 0 (prompt accepted); stderr=%s", exit, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Replacing existing secret") {
		t.Errorf("stderr should show overwrite confirmation prompt; got %q", stderr.String())
	}
	c, _ := config.Load()
	if c.Profiles[c.DefaultProfile].APISecret != "sec_replaced_xyz123" {
		t.Errorf("after 'y' prompt, secret = %q, want sec_replaced_xyz123",
			c.Profiles[c.DefaultProfile].APISecret)
	}
}

func TestAuth_Overwrite_TTY_PromptReject(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_seed_abcdefghij"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed exit=%d", exit)
	}

	cmd.SetStdinTTYForTest(true)
	cmd.SetStderrTTYForTest(true)
	cmd.SetAuthConfirmReaderForTest(func() (string, error) { return "n", nil })
	t.Cleanup(cmd.ResetAuthConfirmReaderForTest)

	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_REJECTED_abc456", "--output-format", "json"}, &stdout, &stderr)
	// TTY reject and non-TTY refuse both yield "we did not save because of
	// existing state" — exit code 7 (ErrConflict) in both paths. The `code`
	// field in the JSON envelope is identical too; the message text disambiguates
	// for humans. Round 2 architecture M1.
	if exit != 7 {
		t.Fatalf("exit=%d, want 7 (conflict); stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "conflict" {
		t.Errorf("code=%v, want conflict", env["code"])
	}
	c, _ := config.Load()
	if c.Profiles[c.DefaultProfile].APISecret != "sec_seed_abcdefghij" {
		t.Errorf("after 'n' prompt, original secret was clobbered: %q", c.Profiles[c.DefaultProfile].APISecret)
	}
}

// TestAuth_ProfileFlag_TargetsNamedProfile pins the C1 fix from Round 4
// adversarial review: `urlbox auth --profile <name>` must write the new
// secret into the named profile, NOT silently fall back to default.
//
// Before this fix, `--profile` was ignored entirely by auth — meaning a
// caller intending to set up a non-default profile would unknowingly
// clobber the default profile's secret. With --force, the clobber was
// silent (the overwrite guard fired against the wrong profile name and
// then --force bypassed it).
func TestAuth_ProfileFlag_TargetsNamedProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	// Seed two profiles so default_profile != target. `config profile create`
	// makes the FIRST created profile the default, so create `default` first
	// to anchor it, then create `staging` as the non-default target.
	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "sec_default_seed12"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed default exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exit := cmd.Execute([]string{"config", "profile", "create", "staging"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed staging exit=%d stderr=%s", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"--profile", "staging", "auth", "--api-secret", "sec_staging_xxxxxxx", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d, want 0; stderr=%s; stdout=%s", exit, stderr.String(), stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	if data["profile"] != "staging" {
		t.Errorf("envelope profile=%v, want staging", data["profile"])
	}

	c, _ := config.Load()
	if c.Profiles["staging"].APISecret != "sec_staging_xxxxxxx" {
		t.Errorf("staging.APISecret=%q, want sec_staging_xxxxxxx", c.Profiles["staging"].APISecret)
	}
	// default profile must NOT have been touched.
	if c.Profiles["default"].APISecret != "sec_default_seed12" {
		t.Errorf("default.APISecret was unexpectedly mutated: %q", c.Profiles["default"].APISecret)
	}
}

// TestAuth_ProfileFlag_UnknownProfile_Errors pins Round 4 C1 + Round 7 EE:
// an unknown --profile name must error rather than silently write to
// default. Round 4 closed the silent-clobber (errored with ErrUsage);
// Round 7 EE aligns the envelope shape to ErrNotFound exit 5 with
// command="auth" — same as profile delete/default and config.Resolve.
// Every "user named a profile that doesn't exist" site in the CLI now
// returns the same envelope.
func TestAuth_ProfileFlag_UnknownProfile_Errors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	// Seed a real default profile so we can prove --profile bogus didn't clobber it.
	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_default_xxxxx"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed exit=%d stderr=%s", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	// Even with --force, an unknown profile name must error rather than
	// silently overwrite default. This closes the Round 4 adversarial repro.
	exit := cmd.Execute([]string{"--profile", "NONEXISTENT", "auth", "--api-secret", "sec_attacker_yy", "--force", "--output-format", "json"}, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("--profile NONEXISTENT should exit 5 (not_found); got exit=%d stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "not_found" {
		t.Errorf("code=%v, want not_found", env["code"])
	}
	if !strings.Contains(env["error"].(string), "NONEXISTENT") {
		t.Errorf("error should name the rejected profile; got %q", env["error"])
	}
	if env["command"] != "auth" {
		t.Errorf("command=%v, want auth", env["command"])
	}

	// Verify default profile was NOT clobbered — this is the load-bearing assertion.
	c, _ := config.Load()
	if c.Profiles["default"].APISecret != "sec_default_xxxxx" {
		t.Errorf("default.APISecret was clobbered by bogus --profile: %q", c.Profiles["default"].APISecret)
	}
}

// TestAuth_EnvProfile_TargetsNamedProfile pins parallel behavior for the
// URLBOX_PROFILE env var.
func TestAuth_EnvProfile_TargetsNamedProfile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	// Seed two profiles so default != target, then point URLBOX_PROFILE at
	// the non-default one and verify auth respects it.
	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "sec_default_seed12"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed default exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if exit := cmd.Execute([]string{"config", "profile", "create", "prod"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("seed prod exit=%d stderr=%s", exit, stderr.String())
	}

	t.Setenv("URLBOX_PROFILE", "prod")
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"auth", "--api-secret", "sec_prod_zzzzzzzz"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	c, _ := config.Load()
	if c.Profiles["prod"].APISecret != "sec_prod_zzzzzzzz" {
		t.Errorf("prod.APISecret=%q, want sec_prod_zzzzzzzz", c.Profiles["prod"].APISecret)
	}
	if c.Profiles["default"].APISecret != "sec_default_seed12" {
		t.Errorf("default.APISecret unexpectedly mutated: %q", c.Profiles["default"].APISecret)
	}
}

// TestAuth_APISecretFlag_EmptyValue_Errors pins Round 4 M3: explicit
// `--api-secret ""` previously fell through silently to env/profile,
// which is the worst option for a user trying to test "what happens
// with no auth?". Now it errors loudly.
func TestAuth_APISecretFlag_EmptyValue_Errors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	cmd.SetStdinTTYForTest(false)
	cmd.SetStderrTTYForTest(false)
	t.Cleanup(cmd.ResetStdinTTYForTest)
	t.Cleanup(cmd.ResetStderrTTYForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-secret", "", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("--api-secret \"\" should error; got exit 0, stdout=%s", stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	if !strings.Contains(env["error"].(string), "empty") {
		t.Errorf("error should mention empty; got %q", env["error"])
	}
}
