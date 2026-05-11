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
	// --reveal: round-trip verifies raw value identity (default is now masked
	// per UX I1; the masking behavior gets its own dedicated tests).
	if exit := cmd.Execute([]string{"config", "get", "api_secret", "--reveal", "--output-format", "quiet"}, &stdout, &stderr); exit != 0 {
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
	if got, want := env["hint"], "Run `urlbox auth --api-secret <secret>` to create one."; got != want {
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

func TestConfigSet_DefaultProfile_UnknownName_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {},
		"work":    {},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "default_profile", "ghost"}, &stdout, &stderr)
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
	if env["error"] != "Unknown profile: ghost" {
		t.Errorf("error=%v", env["error"])
	}
	want := `Configured profiles: "default", "work".`
	if env["hint"] != want {
		t.Errorf("hint=%v want=%v", env["hint"], want)
	}

	// Verify default_profile was NOT changed on disk.
	c, err := config.Load()
	must(t, err)
	if c.DefaultProfile != "default" {
		t.Errorf("DefaultProfile changed to %q after rejected set", c.DefaultProfile)
	}
}

func TestConfigSet_DefaultProfile_NoProfiles_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "default_profile", "anything"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1, got %d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["error"] != "No profiles configured" {
		t.Errorf("error=%v", env["error"])
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

func TestProfileCreate_NewProfileWritesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"config", "profile", "create", "staging",
		"--api-host", "https://api-staging.urlbox.com",
		"--api-secret", "stg_xxx",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	must(t, err)
	for _, want := range []string{`"staging"`, `"stg_xxx"`, `"https://api-staging.urlbox.com"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("config missing %q:\n%s", want, string(b))
		}
	}
}

func TestProfileCreate_DuplicateName_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"config", "profile", "create", "staging", "--api-secret", "x"}, &stdout, &stderr)
	stdout.Reset()
	stderr.Reset()

	exit := cmd.Execute([]string{"config", "profile", "create", "staging", "--api-secret", "y"}, &stdout, &stderr)
	if exit != 7 { // ErrConflict
		t.Fatalf("exit=%d, want 7 (conflict)", exit)
	}
	var env map[string]any
	must(t, json.Unmarshal(stdout.Bytes(), &env))
	if env["code"] != "conflict" {
		t.Errorf("code=%v", env["code"])
	}
	if env["error"] != `Profile "staging" already exists` {
		t.Errorf("error=%v", env["error"])
	}
}

func TestProfileList_ShowsNames_MarksDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "s1"}, new(bytes.Buffer), new(bytes.Buffer))
	cmd.Execute([]string{"config", "profile", "create", "staging", "--api-secret", "s2"}, new(bytes.Buffer), new(bytes.Buffer))

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "list", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	var env map[string]any
	must(t, json.Unmarshal(stdout.Bytes(), &env))
	data, _ := env["data"].(map[string]any)
	profiles, _ := data["profiles"].([]any)
	if len(profiles) != 2 {
		t.Fatalf("profiles len=%d, want 2", len(profiles))
	}
	if data["default"] != "default" {
		t.Errorf("default=%v", data["default"])
	}
}

func TestProfileDefault_SetExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "s1"}, new(bytes.Buffer), new(bytes.Buffer))
	cmd.Execute([]string{"config", "profile", "create", "staging", "--api-secret", "s2"}, new(bytes.Buffer), new(bytes.Buffer))

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "default", "staging"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	c, err := config.Load()
	must(t, err)
	if c.DefaultProfile != "staging" {
		t.Errorf("DefaultProfile=%q", c.DefaultProfile)
	}
}

func TestProfileDefault_Unknown_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "default", "ghost"}, &stdout, &stderr)
	if exit != 5 { // ErrNotFound
		t.Fatalf("exit=%d, want 5 (not_found)", exit)
	}
	var env map[string]any
	must(t, json.Unmarshal(stdout.Bytes(), &env))
	if env["error"] != `Profile "ghost" does not exist` {
		t.Errorf("error=%v", env["error"])
	}
}

func TestProfileDelete_NonDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "s1"}, new(bytes.Buffer), new(bytes.Buffer))
	cmd.Execute([]string{"config", "profile", "create", "staging", "--api-secret", "s2"}, new(bytes.Buffer), new(bytes.Buffer))

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "delete", "staging"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	c, err := config.Load()
	must(t, err)
	if _, ok := c.Profiles["staging"]; ok {
		t.Errorf("staging not deleted")
	}
}

func TestProfileDelete_DefaultProfile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "s1"}, new(bytes.Buffer), new(bytes.Buffer))
	cmd.Execute([]string{"config", "profile", "create", "staging", "--api-secret", "s2"}, new(bytes.Buffer), new(bytes.Buffer))

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "delete", "default"}, &stdout, &stderr)
	if exit != 7 { // ErrConflict
		t.Fatalf("exit=%d, want 7 (conflict)", exit)
	}
	var env map[string]any
	must(t, json.Unmarshal(stdout.Bytes(), &env))
	if env["error"] != `Cannot delete the default profile "default"` {
		t.Errorf("error=%v", env["error"])
	}
	if env["hint"] != "Run 'urlbox config profile default <other>' to switch the default first." {
		t.Errorf("hint=%v", env["hint"])
	}
}

func TestProfileDelete_OnlyProfile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cmd.Execute([]string{"config", "profile", "create", "default", "--api-secret", "s1"}, new(bytes.Buffer), new(bytes.Buffer))

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "delete", "default"}, &stdout, &stderr)
	if exit != 7 {
		t.Fatalf("exit=%d", exit)
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

// TestConfigGet_APISecret_MaskedByDefault pins UX I1: `config get api_secret`
// masks the raw value to prevent accidental scrollback / log / clipboard
// leakage. `config profile list` already masks; `config get api_secret`
// was the outlier.
func TestConfigGet_APISecret_MaskedByDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_supersecretvalue12"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("auth seed exit=%d stderr=%s", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"config", "get", "api_secret", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	val, _ := data["value"].(string)
	if strings.Contains(val, "supersecretvalue") {
		t.Errorf("config get api_secret leaked unmasked value: %q", val)
	}
	if !strings.Contains(val, "sec_") || !strings.Contains(val, "…") {
		t.Errorf("expected masked form (prefix + ellipsis); got %q", val)
	}
	summary, _ := env["summary"].(string)
	if strings.Contains(summary, "supersecretvalue") {
		t.Errorf("summary leaked unmasked value: %q", summary)
	}
}

// TestConfigGet_APISecret_RevealFlag pins the explicit opt-in: --reveal
// prints the raw secret. Use for clipboard-copy workflows, with eyes-on
// awareness.
func TestConfigGet_APISecret_RevealFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_reveal_target_12"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("auth seed exit=%d stderr=%s", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"config", "get", "api_secret", "--reveal", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	val, _ := data["value"].(string)
	if val != "sec_reveal_target_12" {
		t.Errorf("--reveal should print raw secret; got %q", val)
	}
}

// TestConfigGet_APISecret_QuietMode_AlsoMasks pins the quiet-mode path:
// even raw-value emission masks by default. The user paid attention to
// --output-format quiet, but did NOT pass --reveal, so we still protect.
func TestConfigGet_APISecret_QuietMode_AlsoMasks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"auth", "--api-secret", "sec_quietmasked_xyz"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("auth seed exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"config", "get", "api_secret", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "quietmasked_xyz") {
		t.Errorf("quiet mode leaked unmasked value: %q", out)
	}
}

// TestConfigGet_APIKey_NotMasked pins that api_key (publishable) is NOT
// affected by the masking — it's, by definition, publishable.
func TestConfigGet_APIKey_NotMasked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	var stdout, stderr bytes.Buffer
	if exit := cmd.Execute([]string{"config", "profile", "create", "p1", "--api-key", "pub_abcdef123", "--api-secret", "sec_xxxxxxxxxxxx"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("profile create exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit := cmd.Execute([]string{"config", "get", "api_key", "--profile", "p1", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	val, _ := data["value"].(string)
	if val != "pub_abcdef123" {
		t.Errorf("api_key should be raw (it's publishable); got %q", val)
	}
}
