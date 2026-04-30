package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
)

func TestAuth_RequiresAPIKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on missing --api-key")
	}
}

func TestAuth_WritesConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-key", "sec_xxxxxxxxxxxx"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	if got := config.ResolveAPIKey(); got != "sec_xxxxxxxxxxxx" {
		t.Fatalf("key not persisted; got %q", got)
	}
}

func TestAuth_OutputEnvelopeMasksKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-key", "sec_supersecretvalue", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nout: %s", err, stdout.String())
	}
	summary, _ := env["summary"].(string)
	if strings.Contains(summary, "supersecretvalue") {
		t.Fatalf("summary should mask the key: %q", summary)
	}
	if !strings.Contains(summary, "sec_") {
		t.Fatalf("summary should show prefix: %q", summary)
	}
	data, _ := env["data"].(map[string]any)
	if mk, _ := data["masked_key"].(string); strings.Contains(mk, "supersecretvalue") {
		t.Fatalf("masked_key leaked secret: %q", mk)
	}
}

func TestAuth_RejectsEmptyKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-key", ""}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on empty key")
	}
}

func TestAuth_HasBreadcrumbs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"auth", "--api-key", "sec_test1234", "--output-format", "json"}, &stdout, &stderr)

	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Fatalf("expected breadcrumbs, got: %v", env["breadcrumbs"])
	}
}

func TestAuth_NonInteractive_NoKey_StillUsageError(t *testing.T) {
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
	if env["error"] != "missing --api-key" {
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
	if c.Profiles["default"].APIKey != "sec_prompt" {
		t.Errorf("APIKey = %q", c.Profiles["default"].APIKey)
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
