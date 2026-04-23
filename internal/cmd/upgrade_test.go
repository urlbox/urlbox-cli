package cmd_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/urlbox/cli/internal/cmd"
)

func TestUpgrade_DetectsHomebrew(t *testing.T) {
	method := cmd.DetectInstallMethod("/opt/homebrew/bin/urlbox")
	if method != "brew" {
		t.Errorf("expected 'brew', got %q", method)
	}

	method = cmd.DetectInstallMethod("/usr/local/Cellar/urlbox/1.0.0/bin/urlbox")
	if method != "brew" {
		t.Errorf("expected 'brew' for Cellar path, got %q", method)
	}
}

func TestUpgrade_DetectsGoInstall(t *testing.T) {
	method := cmd.DetectInstallMethod("/Users/user/go/bin/urlbox")
	if method != "go" {
		t.Errorf("expected 'go', got %q", method)
	}
}

func TestUpgrade_DetectsScoop(t *testing.T) {
	method := cmd.DetectInstallMethod(`C:\Users\user\scoop\apps\urlbox\current\urlbox.exe`)
	if method != "scoop" {
		t.Errorf("expected 'scoop', got %q", method)
	}
}

func TestUpgrade_DetectsUnknown(t *testing.T) {
	method := cmd.DetectInstallMethod("/usr/local/bin/urlbox")
	if method != "unknown" {
		t.Errorf("expected 'unknown', got %q", method)
	}
}

func TestUpgrade_HelpText(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := cmd.Execute([]string{"upgrade", "--help"}, stdout, stderr)

	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "upgrade") {
		t.Errorf("expected help to mention 'upgrade', got %q", out)
	}
}

func TestUpgrade_DetectsLinuxbrewPath(t *testing.T) {
	method := cmd.DetectInstallMethod("/home/linuxbrew/.linuxbrew/bin/urlbox")
	if method != "brew" {
		t.Errorf("expected 'brew' for linuxbrew path, got %q", method)
	}
}

func TestUpgrade_CaseInsensitive(t *testing.T) {
	method := cmd.DetectInstallMethod("/OPT/HOMEBREW/BIN/URLBOX")
	if method != "brew" {
		t.Errorf("expected case-insensitive brew detection, got %q", method)
	}
}

// fakeExec records the command that was executed.
type fakeExec struct {
	name string
	args []string
	err  error // error to return when called
}

func (f *fakeExec) run(_ io.Writer, name string, args ...string) error {
	f.name = name
	f.args = args
	return f.err
}

func TestRunUpgrade_BrewPath(t *testing.T) {
	fake := &fakeExec{}
	stderr := &bytes.Buffer{}

	err := cmd.RunUpgrade(stderr, "/opt/homebrew/bin/urlbox", fake.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.name != "brew" {
		t.Errorf("expected command 'brew', got %q", fake.name)
	}
	if len(fake.args) < 2 || fake.args[0] != "upgrade" || fake.args[1] != "urlbox/tap/urlbox" {
		t.Errorf("expected args [upgrade urlbox/tap/urlbox], got %v", fake.args)
	}
	out := stderr.String()
	if !strings.Contains(out, "Homebrew") {
		t.Errorf("expected stderr to mention Homebrew, got %q", out)
	}
}

func TestRunUpgrade_ScoopPath(t *testing.T) {
	fake := &fakeExec{}
	stderr := &bytes.Buffer{}

	err := cmd.RunUpgrade(stderr, `C:\Users\user\scoop\apps\urlbox\current\urlbox.exe`, fake.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.name != "scoop" {
		t.Errorf("expected command 'scoop', got %q", fake.name)
	}
	if len(fake.args) < 2 || fake.args[0] != "update" || fake.args[1] != "urlbox" {
		t.Errorf("expected args [update urlbox], got %v", fake.args)
	}
}

func TestRunUpgrade_GoPath(t *testing.T) {
	fake := &fakeExec{}
	stderr := &bytes.Buffer{}

	err := cmd.RunUpgrade(stderr, "/Users/user/go/bin/urlbox", fake.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.name != "go" {
		t.Errorf("expected command 'go', got %q", fake.name)
	}
	if len(fake.args) < 2 || fake.args[0] != "install" {
		t.Errorf("expected args starting with 'install', got %v", fake.args)
	}
}

func TestRunUpgrade_UnknownPath_ShowsManualInstructions(t *testing.T) {
	fake := &fakeExec{}
	stderr := &bytes.Buffer{}

	err := cmd.RunUpgrade(stderr, "/usr/local/bin/urlbox", fake.run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should NOT have called any external command
	if fake.name != "" {
		t.Errorf("expected no command to be run, got %q", fake.name)
	}
	out := stderr.String()
	if !strings.Contains(out, "Could not detect") {
		t.Errorf("expected manual instructions, got %q", out)
	}
	if !strings.Contains(out, "brew upgrade") {
		t.Errorf("expected brew instruction in manual help, got %q", out)
	}
	if !strings.Contains(out, "curl") {
		t.Errorf("expected curl instruction in manual help, got %q", out)
	}
}

func TestRunUpgrade_ShowsVersionAndMethod(t *testing.T) {
	fake := &fakeExec{}
	stderr := &bytes.Buffer{}

	_ = cmd.RunUpgrade(stderr, "/opt/homebrew/bin/urlbox", fake.run)

	out := stderr.String()
	if !strings.Contains(out, "Current version:") {
		t.Errorf("expected 'Current version:' in output, got %q", out)
	}
	if !strings.Contains(out, "Install method:") {
		t.Errorf("expected 'Install method:' in output, got %q", out)
	}
	if !strings.Contains(out, "Binary path:") {
		t.Errorf("expected 'Binary path:' in output, got %q", out)
	}
}

func TestRunUpgrade_ExecutorError(t *testing.T) {
	fake := &fakeExec{err: fmt.Errorf("brew not found")}
	stderr := &bytes.Buffer{}

	err := cmd.RunUpgrade(stderr, "/opt/homebrew/bin/urlbox", fake.run)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "brew not found") {
		t.Errorf("expected 'brew not found' error, got %q", err.Error())
	}
}
