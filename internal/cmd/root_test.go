package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/cli/internal/cmd"
)

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

	errOut := stderr.String()
	if !strings.Contains(errOut, "unknown") && !strings.Contains(errOut, "nonexistent") {
		t.Errorf("expected error message about unknown command, got %q", errOut)
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
