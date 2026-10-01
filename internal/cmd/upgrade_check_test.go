package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

// runUpgradeJSON runs `urlbox upgrade` as if installed at execPath, with
// the given release answer, and decodes the JSON envelope.
func runUpgradeJSON(t *testing.T, execPath string, rel *fakeRelease, fake *fakeExec) (env map[string]any, stderr string, code int) {
	t.Helper()
	cmd.SetUpdateCheckForTest("1.2.0", rel.fetch)
	t.Cleanup(cmd.ResetUpdateCheckForTest)
	cmd.SetUpgradeForTest(execPath, fake.run)
	t.Cleanup(cmd.ResetUpgradeForTest)

	var out, errb bytes.Buffer
	code = cmd.Execute([]string{"upgrade", "--output-format", "json"}, &out, &errb)
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, out.String())
	}
	return env, errb.String(), code
}

func TestUpgrade_AlreadyUpToDate_SkipsPackageManager(t *testing.T) {
	fake := &fakeExec{}
	env, stderr, code := runUpgradeJSON(t, "/opt/homebrew/bin/urlbox", &fakeRelease{latest: "1.2.0"}, fake)

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if fake.name != "" {
		t.Errorf("package manager must not run when current; ran %q %v", fake.name, fake.args)
	}
	data, _ := env["data"].(map[string]any)
	if data["upToDate"] != true || data["latestVersion"] != "1.2.0" {
		t.Errorf("data = %v", data)
	}
	if !strings.Contains(stderr, "already up to date") {
		t.Errorf("stderr should say it's up to date: %q", stderr)
	}
}

func TestUpgrade_NewerRelease_RunsPackageManagerAndNamesTarget(t *testing.T) {
	fake := &fakeExec{}
	env, stderr, code := runUpgradeJSON(t, "/opt/homebrew/bin/urlbox", &fakeRelease{latest: "1.3.0"}, fake)

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if fake.name != "brew" {
		t.Errorf("expected brew to run, got %q", fake.name)
	}
	data, _ := env["data"].(map[string]any)
	if data["upToDate"] != false || data["latestVersion"] != "1.3.0" || data["currentVersion"] != "1.2.0" {
		t.Errorf("data = %v", data)
	}
	if summary, _ := env["summary"].(string); summary != "Upgraded 1.2.0 → 1.3.0 via Homebrew" {
		t.Errorf("summary = %q", summary)
	}
	if !strings.Contains(stderr, "1.2.0 → 1.3.0") {
		t.Errorf("stderr should name the target version: %q", stderr)
	}
}

func TestUpgrade_CheckFails_StillUpgrades(t *testing.T) {
	fake := &fakeExec{}
	env, stderr, code := runUpgradeJSON(t, "/opt/homebrew/bin/urlbox", &fakeRelease{err: errors.New("rate limited")}, fake)

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if fake.name != "brew" {
		t.Errorf("a failed check must not block the upgrade; ran %q", fake.name)
	}
	if !strings.Contains(stderr, "Couldn't check for the latest version") {
		t.Errorf("stderr should mention the failed check: %q", stderr)
	}
	data, _ := env["data"].(map[string]any)
	if _, ok := data["latestVersion"]; ok {
		t.Errorf("latestVersion should be absent when unknown: %v", data)
	}
	if summary, _ := env["summary"].(string); summary != "Upgraded via Homebrew" {
		t.Errorf("summary = %q", summary)
	}
}

func TestUpgrade_FollowsSymlinkToDetectInstallMethod(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "Cellar", "urlbox", "1.2.0", "bin", "urlbox")
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("bin"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "bin", "urlbox") // Intel Homebrew: /usr/local/bin/urlbox → Cellar
	if err := os.MkdirAll(filepath.Dir(link), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	fake := &fakeExec{}
	_, stderr, code := runUpgradeJSON(t, link, &fakeRelease{latest: "1.3.0"}, fake)

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if fake.name != "brew" {
		t.Errorf("symlinked Homebrew install should upgrade via brew; ran %q", fake.name)
	}
}
