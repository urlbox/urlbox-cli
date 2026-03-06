package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSetAndGetProfileScopedAPIHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	exitCode := runConfig([]string{
		"set",
		"api_host",
		"https://api-staging.urlbox.com",
		"--profile", "staging",
	})
	if exitCode != 0 {
		t.Fatalf("unexpected exit code from config set: %d", exitCode)
	}

	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runConfig([]string{
			"get",
			"api_host",
			"--profile", "staging",
			"--output-format", "json",
		})
	})
	if exitCode != 0 {
		t.Fatalf("unexpected exit code from config get: %d\noutput:\n%s", exitCode, output)
	}
	if !strings.Contains(output, "\"value\": \"https://api-staging.urlbox.com\"") {
		t.Fatalf("expected api_host in output, got:\n%s", output)
	}

	configPath := filepath.Join(home, ".config", "urlbox", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file at %s: %v", configPath, err)
	}
}

func TestConfigProfileDeleteRequiresYes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if exitCode := runConfig([]string{
		"profile", "create", "staging",
		"--api-host", "https://api-staging.urlbox.com",
	}); exitCode != 0 {
		t.Fatalf("unexpected exit code creating profile: %d", exitCode)
	}

	output, exitCode := captureStdoutAndStderr(t, func() int {
		return runConfig([]string{"profile", "delete", "staging"})
	})
	if exitCode == 0 {
		t.Fatalf("expected failure when deleting without --yes")
	}
	if !strings.Contains(output, "deletion requires --yes") {
		t.Fatalf("expected confirmation error, got:\n%s", output)
	}
}
