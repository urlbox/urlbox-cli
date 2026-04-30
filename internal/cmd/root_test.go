package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestRoot_HasProfileFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "--profile") {
		t.Error("--help is missing --profile")
	}
	if strings.Contains(help, "--env") {
		t.Error("--help contains --env; the flag was dropped in Phase 3")
	}
}

func TestRootCommand_VersionFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--version"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox") {
		t.Errorf("expected version output to contain 'urlbox', got %q", out)
	}
}

func TestRootCommand_Help(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox") {
		t.Errorf("expected help output to contain 'urlbox', got %q", out)
	}
	if !strings.Contains(out, "Usage") && !strings.Contains(out, "usage") {
		t.Errorf("expected help output to contain usage info, got %q", out)
	}
}

func TestRootCommand_UnknownSubcommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"nonexistent"}, stdout, stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown subcommand")
	}
	// Error should be in the envelope on stdout (not stderr)
	out := stdout.String()
	if !strings.Contains(out, "unknown") || !strings.Contains(out, "nonexistent") {
		t.Errorf("expected error about unknown command on stdout, got %q", out)
	}
}

func TestRootCommand_NoArgs_ShowsHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox") {
		t.Errorf("expected help output to contain 'urlbox', got %q", out)
	}
}

func TestRootCommand_VersionFormat(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--version"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "commit:") {
		t.Errorf("expected version to contain 'commit:', got %q", out)
	}
	if !strings.Contains(out, "built:") {
		t.Errorf("expected version to contain 'built:', got %q", out)
	}
}

func TestRootCommand_HelpGoesToStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"--help"}, stdout, stderr)
	if stdout.Len() == 0 {
		t.Error("expected help output on stdout, got nothing")
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no output on stderr for --help, got %q", stderr.String())
	}
}

func TestRootCommand_ErrorGoesToStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"nonexistent"}, stdout, stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.Len() == 0 {
		t.Error("expected error output on stdout, got nothing")
	}
}

func TestRootCommand_OutputFormatFlag_Accepted(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--output-format", "json", "--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
}

func TestRootCommand_UnknownSubcommand_JSONErrorEnvelope(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--output-format", "json", "nonexistent"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if env["ok"] != false {
		t.Errorf("expected ok=false, got %v", env["ok"])
	}
	if env["code"] != "usage" {
		t.Errorf("expected code=usage, got %v", env["code"])
	}
}

func TestRootCommand_UnknownSubcommand_ExitCode1(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"nonexistent"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 (usage), got %d", code)
	}
}

func TestRootCommand_ErrorEnvelope_GoesToStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"--output-format", "json", "nonexistent"}, stdout, stderr)
	if stdout.Len() == 0 {
		t.Error("expected error envelope on stdout, got nothing")
	}
}

func TestRoot_NoArgs_PrintsBannerToStderr_WhenTTY(t *testing.T) {
	cmd.SetStderrTTYForTest(true)
	defer cmd.ResetStderrTTYForTest()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{}, stdout, stderr)

	if !strings.Contains(stderr.String(), "urlbox commands") {
		t.Fatalf("expected banner on stderr; got: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "If you're looking") {
		t.Fatalf("banner must not appear on stdout; got: %q", stdout.String())
	}
}

func TestRoot_NoArgs_NoBanner_WhenPiped(t *testing.T) {
	cmd.SetStderrTTYForTest(false)
	defer cmd.ResetStderrTTYForTest()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{}, stdout, stderr)

	if strings.Contains(stderr.String(), "If you're looking") {
		t.Fatalf("banner should not show in non-TTY; got: %q", stderr.String())
	}
}

func TestRoot_WithSubcommand_NoBanner(t *testing.T) {
	cmd.SetStderrTTYForTest(true)
	defer cmd.ResetStderrTTYForTest()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)

	if strings.Contains(stderr.String(), "If you're looking") {
		t.Fatalf("banner leaked into subcommand run; got: %q", stderr.String())
	}
}
