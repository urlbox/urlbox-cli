package cmd_test

import (
	"bytes"
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
