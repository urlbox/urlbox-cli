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

func TestConfigPath_PrintsResolvedPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "path", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	want := filepath.Join(dir, "urlbox", "config.json")
	if got := strings.TrimSpace(stdout.String()); got != strconvQuote(want) {
		t.Errorf("path = %s, want %s", got, strconvQuote(want))
	}
}

func TestConfigPath_JSONEnvelope(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "path", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["command"] != "config path" {
		t.Errorf("command=%v", env["command"])
	}
	data, _ := env["data"].(map[string]any)
	if data["path"] != filepath.Join(dir, "urlbox", "config.json") {
		t.Errorf("path=%v", data["path"])
	}
}

// seedConfig writes a minimal valid config with the given profiles, sets default_profile
// to the alphabetically-first profile name, and returns the resolved file path.
func seedConfig(t *testing.T, dir string, profiles map[string]config.Profile) string {
	t.Helper()
	cfgDir := filepath.Join(dir, "urlbox")
	must(t, os.MkdirAll(cfgDir, 0o700))
	path := filepath.Join(cfgDir, "config.json")

	first := ""
	for name := range profiles {
		if first == "" || name < first {
			first = name
		}
	}
	c := &config.Config{DefaultProfile: first, Profiles: profiles}
	b, err := json.MarshalIndent(c, "", "  ")
	must(t, err)
	must(t, os.WriteFile(path, b, 0o600))
	return path
}

func TestConfigSet_Get_RoundTrip_SingleProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{"default": {}})

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_xxx"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("set: exit=%d stderr=%s", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := cmd.Execute([]string{"config", "get", "api_secret", "--output-format", "quiet"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("get: exit=%d", exit)
	}
	got := strings.TrimSpace(stdout.String())
	if got != strconvQuote("sec_xxx") {
		t.Errorf("get = %s, want %s", got, strconvQuote("sec_xxx"))
	}

	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	must(t, err)
	if !strings.Contains(string(b), `"api_secret": "sec_xxx"`) {
		t.Errorf("file missing api_secret:\n%s", string(b))
	}
	if !strings.Contains(string(b), `"profiles"`) {
		t.Errorf("file missing profiles:\n%s", string(b))
	}
}

func TestConfigSet_NoProfiles_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_xxx"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v", env["code"])
	}
	if env["error"] != "No profiles configured" {
		t.Errorf("error=%v", env["error"])
	}
	if got, want := env["hint"], "Run `urlbox auth --api-key <secret>` to create one."; got != want {
		t.Errorf("hint=%v want=%v", got, want)
	}
}

func TestConfigSet_MultipleProfiles_NoFlag_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APIKey: "k1"},
		"work":    {APIKey: "k2"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_xxx"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1, got %d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v", env["code"])
	}
	if env["error"] != "--profile is required" {
		t.Errorf("error=%v", env["error"])
	}
	want := `Configured profiles: "default", "work". Add --profile <name> to choose one.`
	if env["hint"] != want {
		t.Errorf("hint=%v want=%v", env["hint"], want)
	}
}

func TestConfigSet_MultipleProfiles_WithFlag_Writes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APIKey: "k1"},
		"work":    {APIKey: "k2"},
	})

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"--profile", "work", "config", "set", "api_secret", "sec_work"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("set: exit=%d stderr=%s", exit, stderr.String())
	}

	c, err := config.Load()
	must(t, err)
	if got := c.Profiles["work"].APISecret; got != "sec_work" {
		t.Errorf("work.api_secret = %q, want sec_work", got)
	}
	if got := c.Profiles["default"].APISecret; got != "" {
		t.Errorf("default.api_secret = %q, want empty (untouched)", got)
	}
}

func TestConfigSet_DefaultProfileKey_AlwaysWrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {},
		"work":    {},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "default_profile", "work"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	c, err := config.Load()
	must(t, err)
	if c.DefaultProfile != "work" {
		t.Errorf("DefaultProfile=%q, want work", c.DefaultProfile)
	}
}

func TestConfigSet_UnknownKey_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "favorite_color", "blue"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v", env["code"])
	}
	if env["error"] != "Unknown config key: favorite_color" {
		t.Errorf("error=%v", env["error"])
	}
	if env["hint"] != "Supported: api_key, api_secret, api_host, default_profile" {
		t.Errorf("hint=%v", env["hint"])
	}
}

func TestConfigGet_MissingValue_ReturnsEmptyString(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{"default": {}})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "get", "api_key", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if got := strings.TrimSpace(stdout.String()); got != strconvQuote("") {
		t.Errorf("got %s, want %s", got, strconvQuote(""))
	}
}

// strconvQuote wraps a string in JSON-escaped double quotes (matches what
// QuietFormatter / jsonScalarLine emit for a string scalar).
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
