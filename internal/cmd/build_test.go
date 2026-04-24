package cmd_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuild_LdflagsInjectVersion(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping build integration test in CI (covered by Makefile)")
	}

	// Build the binary with custom ldflags
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "urlbox")

	ldflags := strings.Join([]string{
		"-X github.com/urlbox/urlbox-cli/internal/version.Version=1.2.3-test",
		"-X github.com/urlbox/urlbox-cli/internal/version.Commit=abc1234",
		"-X github.com/urlbox/urlbox-cli/internal/version.Date=2026-01-01T00:00:00Z",
	}, " ")

	//nolint:gosec // test builds the project binary with known-safe arguments
	build := exec.CommandContext(context.Background(), "go", "build", "-ldflags", ldflags, "-o", binary, "./cmd/urlbox")
	build.Dir = findRepoRoot(t)
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// Run the binary with --version
	//nolint:gosec // test executes the binary it just built
	run := exec.CommandContext(context.Background(), binary, "--version")
	versionOut, err := run.Output()
	if err != nil {
		t.Fatalf("binary --version failed: %v", err)
	}

	version := string(versionOut)
	if !strings.Contains(version, "1.2.3-test") {
		t.Errorf("expected version to contain '1.2.3-test', got %q", version)
	}
	if !strings.Contains(version, "abc1234") {
		t.Errorf("expected version to contain commit 'abc1234', got %q", version)
	}
	if !strings.Contains(version, "2026-01-01") {
		t.Errorf("expected version to contain build date '2026-01-01', got %q", version)
	}
}

func TestBuild_GoReleaserConfigValid(t *testing.T) {
	if _, err := exec.LookPath("goreleaser"); err != nil {
		t.Skip("goreleaser not installed, skipping config validation")
	}

	check := exec.CommandContext(context.Background(), "goreleaser", "check") //nolint:gosec // test runs goreleaser with known-safe args
	check.Dir = findRepoRoot(t)
	out, err := check.CombinedOutput()
	if err != nil {
		t.Fatalf("goreleaser check failed: %v\n%s", err, out)
	}
}

func TestBuild_InstallScriptSyntax(t *testing.T) {
	root := findRepoRoot(t)
	script := filepath.Join(root, "scripts", "install.sh")

	if _, err := os.Stat(script); os.IsNotExist(err) {
		t.Fatal("scripts/install.sh does not exist")
	}

	// sh -n does a syntax check without executing
	check := exec.CommandContext(context.Background(), "sh", "-n", script) //nolint:gosec // test runs sh syntax check on project file
	out, err := check.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh has syntax errors: %v\n%s", err, out)
	}
}

// findRepoRoot walks up from the test file to find the repo root (where go.mod lives).
func findRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}
