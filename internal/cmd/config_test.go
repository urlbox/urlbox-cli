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
	// The hint must name BOTH onboarding paths. `login` needs a browser, so a
	// headless box pointed only at `login` has nowhere to go; `auth` is the
	// route that works there.
	hint, _ := env["hint"].(string)
	for _, want := range []string{"urlbox login", "urlbox auth"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint must mention %q for the headless path; got %q", want, hint)
		}
	}
}

// Historical TestConfigSet_MultipleProfiles_NoFlag_Errors asserted that
// 2+ profiles always errored without --profile. Round 5 CI-2 changed
// that to honor default_profile transparently (matching how render /
// status / link resolve). Coverage moved to:
//   - TestConfigSet_MultipleProfiles_UsesDefaultProfile (positive)
//   - TestConfigSet_MultipleProfiles_NoDefaultProfile_Errors (fallback)
// — both at the bottom of this file.

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

// TestConfigSet_DefaultProfile_UnknownName_Errors pins Round 7 EE class-fix:
// naming a non-existent profile (here `ghost`) returns ErrNotFound exit 5,
// not the legacy ErrUsage exit 1. This aligns with `profile default`/
// `profile delete` and with the unified config.Resolve for render/status/
// link/doctor — every "user named a profile that doesn't exist" site
// across the CLI now reports the same envelope shape.
func TestConfigSet_DefaultProfile_UnknownName_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {},
		"work":    {},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "default_profile", "ghost"}, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("expected exit 5 (not_found), got %d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "not_found" {
		t.Errorf("code=%v, want not_found", env["code"])
	}
	if env["error"] != `Profile "ghost" does not exist` {
		t.Errorf("error=%v", env["error"])
	}
	if env["command"] != "config set" {
		t.Errorf("command=%v, want config set", env["command"])
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
	if env["hint"] != "Supported: api_key, api_secret, api_host, default_profile, session_token, active_org, active_project" {
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
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_supersecretvalue12"},
	})

	var stdout, stderr bytes.Buffer
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
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_reveal_target_12"},
	})

	var stdout, stderr bytes.Buffer
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
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_quietmasked_xyz"},
	})

	var stdout, stderr bytes.Buffer
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

// TestConfigSet_APISecret_MaskedInEnvelope pins Round 4 M4: `config set
// api_secret <value>` echoed the raw secret back in .data.value and in
// the summary string. UX I1 (Round 1) only fixed `config get`. The set
// path still leaked into CI logs and terminal scrollback.
func TestConfigSet_APISecret_MaskedInEnvelope(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{"default": {}})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_leaktest_abcdef", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	val, _ := data["value"].(string)
	if strings.Contains(val, "leaktest_abcdef") {
		t.Errorf("envelope .data.value leaked raw secret: %q", val)
	}
	if !strings.Contains(val, "sec_") || !strings.Contains(val, "…") {
		t.Errorf("expected masked form (prefix + ellipsis); got %q", val)
	}
	summary, _ := env["summary"].(string)
	if strings.Contains(summary, "leaktest_abcdef") {
		t.Errorf("summary leaked raw secret: %q", summary)
	}

	// Sanity check: the FILE actually has the raw secret saved (we mask
	// the envelope, not the storage).
	b, _ := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if !strings.Contains(string(b), "sec_leaktest_abcdef") {
		t.Errorf("config file is missing the raw secret (set should still persist it):\n%s", string(b))
	}
}

// TestConfigSet_APIKey_NotMasked pins that api_key (publishable) is NOT
// affected by the masking — same contract as `config get`.
func TestConfigSet_APIKey_NotMasked(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{"default": {}})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_key", "pub_visible_abc", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	if data["value"] != "pub_visible_abc" {
		t.Errorf("api_key should be raw (publishable); got %v", data["value"])
	}
}

// TestConfigGet_UnknownFlagProfile_Errors pins Round 7 EE class-fix:
// `config get --profile X` where X doesn't exist returns ErrNotFound
// exit 5 with command="config get", matching profile delete/default +
// config.Resolve (used by render/status/link/doctor). Pre-fix this
// returned ErrUsage exit 1 with command="" — the original Round 7
// finding ("envelope shape misalignment").
func TestConfigGet_UnknownFlagProfile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xxxxxx"},
		"work":    {APISecret: "sec_work_yyyyyyy"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--profile", "ghost", "config", "get", "api_secret", "--output-format", "json"}, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("--profile ghost should exit 5 (not_found); got exit %d stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "not_found" {
		t.Errorf("code=%v, want not_found", env["code"])
	}
	if !strings.Contains(env["error"].(string), "ghost") {
		t.Errorf("error should name the rejected profile; got %q", env["error"])
	}
	if env["command"] != "config get" {
		t.Errorf("command=%v, want config get", env["command"])
	}
	// Verify no secret leaked.
	if strings.Contains(stdout.String(), "sec_default_xxxxxx") || strings.Contains(stdout.String(), "sec_work_yyyyyyy") {
		t.Errorf("envelope leaked a profile secret on unknown --profile: %s", stdout.String())
	}
}

// TestConfigSet_UnknownFlagProfile_Errors mirrors the get-side test —
// `config set --profile X api_key Y` for an unknown X is the same
// class and returns the same envelope shape.
func TestConfigSet_UnknownFlagProfile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xxxxxx"},
		"work":    {APISecret: "sec_work_yyyyyyy"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--profile", "ghost", "config", "set", "api_key", "pub_xxx", "--output-format", "json"}, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("--profile ghost should exit 5 (not_found); got exit %d stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "not_found" {
		t.Errorf("code=%v, want not_found", env["code"])
	}
	if env["command"] != "config set" {
		t.Errorf("command=%v, want config set", env["command"])
	}
	// Verify on-disk state was not mutated.
	c, _ := config.Load()
	if c.Profiles["default"].APIKey != "" || c.Profiles["work"].APIKey != "" {
		t.Errorf("config was mutated despite rejected --profile: %+v", c.Profiles)
	}
}

// TestConfigGet_UnknownEnvProfile_Errors pins Round 5 Adv-2 +
// Round 7 EE: a typo in URLBOX_PROFILE silently fell back to the
// default profile, leaking the default's secret. Round 5 closed the
// silent-fallback (correctly errored with ErrUsage). Round 7 EE
// aligns the envelope to ErrNotFound exit 5 with command="config get"
// — same shape as profile delete/default and the unified
// config.Resolve. The class is "user named a profile that doesn't
// exist", which is a not_found regardless of which source (flag/env)
// supplied the name.
func TestConfigGet_UnknownEnvProfile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Single profile is the exact code path the adversarial repro hit:
	// resolveTargetProfile auto-selects the lone profile and ignores
	// URLBOX_PROFILE entirely, so an env-var typo silently leaks the
	// wrong (default) profile's data.
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xxxxxx"},
	})
	t.Setenv("URLBOX_PROFILE", "ghost")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "get", "api_secret", "--output-format", "json"}, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("URLBOX_PROFILE=ghost should exit 5 (not_found); got exit %d stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "not_found" {
		t.Errorf("code=%v, want not_found", env["code"])
	}
	if !strings.Contains(env["error"].(string), "ghost") {
		t.Errorf("error should name the rejected profile; got %q", env["error"])
	}
	if env["command"] != "config get" {
		t.Errorf("command=%v, want config get", env["command"])
	}
	// Verify no secret leaked through the silent-fallback path.
	if strings.Contains(stdout.String(), "sec_default_xxxxxx") || strings.Contains(stdout.String(), "sec_work_yyyyyy") {
		t.Errorf("envelope leaked a profile secret on unknown URLBOX_PROFILE: %s", stdout.String())
	}
}

// TestConfigGet_ValidEnvProfile_TargetsThatProfile pins the positive case:
// URLBOX_PROFILE=work selects the work profile (now that we don't silently
// fall back).
func TestConfigGet_ValidEnvProfile_TargetsThatProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Two profiles with default set; URLBOX_PROFILE should override
	// the default and target work.
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xxxxxx"},
		"work":    {APISecret: "sec_work_yyyyyy"},
	})
	t.Setenv("URLBOX_PROFILE", "work")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "get", "api_secret", "--reveal", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	if data["value"] != "sec_work_yyyyyy" {
		t.Errorf("value=%v, want sec_work_yyyyyy (env-selected profile)", data["value"])
	}
}

// TestConfigProfileCreate_RejectsDangerousNames pins Round 5 Adv-4:
// profile names could contain path separators, control chars, and null
// bytes. Null-byte truncation in particular is a footgun — "a\x00b"
// silently collides with "a" because the JSON store keys by the
// truncated string. Reject all unsafe shapes at creation time.
func TestConfigProfileCreate_RejectsDangerousNames(t *testing.T) {
	cases := []struct {
		name string
		give string
	}{
		{"path traversal", "../evil"},
		{"forward slash", "team/work"},
		{"backslash", `team\work`},
		{"null byte", "a\x00b"},
		{"newline", "foo\nbar"},
		{"carriage return", "foo\rbar"},
		{"tab", "foo\tbar"},
		{"leading space", " foo"},
		{"trailing space", "foo "},
		{"leading dot", ".hidden"},
		{"leading hyphen", "-arg"},
		{"empty string", ""},
		{"too long", strings.Repeat("a", 65)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{"config", "profile", "create", c.give, "--output-format", "json"}, &stdout, &stderr)
			if exit == 0 {
				t.Fatalf("profile create %q should error; got exit 0 stdout=%s", c.give, stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "usage" {
				t.Errorf("code=%v, want usage", env["code"])
			}
		})
	}
}

// TestConfigProfileCreate_AcceptsSafeNames pins the positive cases —
// alphanumerics + underscore + hyphen, max 64 chars, first char must be
// alphanumeric.
func TestConfigProfileCreate_AcceptsSafeNames(t *testing.T) {
	cases := []string{
		"default",
		"work",
		"team-staging",
		"team_x",
		"a", // single char
		"team-staging-prod",
		"P_1",
		strings.Repeat("a", 64), // boundary
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{"config", "profile", "create", name}, &stdout, &stderr)
			if exit != 0 {
				t.Fatalf("profile create %q should succeed; got exit %d stderr=%s", name, exit, stderr.String())
			}
		})
	}
}

// TestConfigSet_MultipleProfiles_UsesDefaultProfile pins Round 5 CI-2:
// when 2+ profiles exist AND default_profile is set, config get/set
// should target the default rather than require --profile. Previously
// a CI script that ran `config set api_key X` after creating a second
// profile started failing with exit 1, inconsistent with how render /
// status / link resolve the default profile.
func TestConfigSet_MultipleProfiles_UsesDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// seedConfig sets DefaultProfile to the alphabetically-first profile,
	// so "default" here is the default.
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APIKey: "k1"},
		"work":    {APIKey: "k2"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_to_default"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("config set should succeed using default_profile; got exit %d stderr=%s", exit, stderr.String())
	}
	c, err := config.Load()
	must(t, err)
	if got := c.Profiles["default"].APISecret; got != "sec_to_default" {
		t.Errorf("default.api_secret=%q, want sec_to_default", got)
	}
	if got := c.Profiles["work"].APISecret; got != "" {
		t.Errorf("work.api_secret=%q, want untouched (empty)", got)
	}
}

// TestConfigGet_MultipleProfiles_UsesDefaultProfile mirrors the above
// for the read side.
func TestConfigGet_MultipleProfiles_UsesDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APIKey: "k_default"},
		"work":    {APIKey: "k_work"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "get", "api_key", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("config get should succeed using default_profile; got exit %d stderr=%s", exit, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	if got != strconvQuote("k_default") {
		t.Errorf("got %s, want %s", got, strconvQuote("k_default"))
	}
}

// TestConfigSet_MultipleProfiles_NoDefaultProfile_Errors keeps the
// ambiguous-error path under coverage: 2+ profiles AND default_profile
// is empty (rare; manually-edited config) is still a usage error.
func TestConfigSet_MultipleProfiles_NoDefaultProfile_Errors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	// Construct the config manually to leave DefaultProfile blank.
	cfgDir := filepath.Join(dir, "urlbox")
	must(t, os.MkdirAll(cfgDir, 0o700))
	c := &config.Config{
		DefaultProfile: "",
		Profiles: map[string]config.Profile{
			"a": {APIKey: "k1"},
			"b": {APIKey: "k2"},
		},
	}
	b, err := json.MarshalIndent(c, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(cfgDir, "config.json"), b, 0o600))

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_xxx", "--output-format", "json"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1 (no default_profile, no --profile); got %d stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["error"] != "--profile is required" {
		t.Errorf("error=%v, want '--profile is required'", env["error"])
	}
	wantHint := `Configured profiles: "a", "b". Add --profile <name> to choose one.`
	if env["hint"] != wantHint {
		t.Errorf("hint=%v want=%v", env["hint"], wantHint)
	}
}

// TestConfigSet_APISecret_RejectsBadValues pins Round 6 Adv-4 + Adv-5:
// config set api_secret silently accepted whitespace-only and empty
// strings, AND its empty-string path silently cleared the saved secret
// (bypassing the auth overwrite guard entirely). Now: validateSecretValue
// is the single gate every secret-writing path uses.
func TestConfigSet_APISecret_RejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"empty string", ""},
		{"spaces only", "   "},
		{"tabs only", "\t\t"},
		{"newlines only", "\n"},
		{"internal newline", "abc\ndef"},
		{"null byte", "abc\x00def"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", dir)
			seedConfig(t, dir, map[string]config.Profile{
				"default": {APISecret: "sec_preserve_me_xx"},
			})

			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{"config", "set", "api_secret", c.val, "--output-format", "json"}, &stdout, &stderr)
			if exit == 0 {
				t.Fatalf("config set api_secret %q should error; got exit 0 stdout=%s", c.val, stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "usage" {
				t.Errorf("code=%v, want usage", env["code"])
			}

			// Critical: the existing secret must NOT have been clobbered.
			cl, _ := config.Load()
			if cl.Profiles["default"].APISecret != "sec_preserve_me_xx" {
				t.Errorf("existing secret was overwritten by invalid input %q: now %q", c.val, cl.Profiles["default"].APISecret)
			}
		})
	}
}

// TestConfigSet_APISecret_OverwriteGuard pins the overwrite guard:
// setting a DIFFERENT secret without --force should refuse. Previously
// config set was the unguarded back door.
func TestConfigSet_APISecret_OverwriteGuard(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_original_abcdef"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_DIFFERENT_xyz", "--output-format", "json"}, &stdout, &stderr)
	if exit != 7 {
		t.Fatalf("exit=%d, want 7 (conflict); stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "conflict" {
		t.Errorf("code=%v, want conflict", env["code"])
	}
	// Critical: no silent overwrite.
	cl, _ := config.Load()
	if cl.Profiles["default"].APISecret != "sec_original_abcdef" {
		t.Errorf("default.APISecret was clobbered without --force: %q", cl.Profiles["default"].APISecret)
	}
}

// TestConfigSet_APISecret_ForceOverwrites pins the CI escape hatch.
func TestConfigSet_APISecret_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_original_abcdef"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_NEW_uvwxyz12", "--force"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	cl, _ := config.Load()
	if cl.Profiles["default"].APISecret != "sec_NEW_uvwxyz12" {
		t.Errorf("--force did not overwrite: %q", cl.Profiles["default"].APISecret)
	}
}

// TestConfigSet_APISecret_SameValueIdempotent pins that re-setting the
// SAME secret is a no-op (no guard fires).
func TestConfigSet_APISecret_SameValueIdempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_same_xyzxyzxy"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "set", "api_secret", "sec_same_xyzxyzxy"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("idempotent re-set should succeed; exit=%d stderr=%s", exit, stderr.String())
	}
}

// TestConfigProfileCreate_RejectsBadSecretValues pins that profile-create
// also goes through validateSecretValue. Whitespace/control chars cannot
// land in a new profile any more than in an existing one.
func TestConfigProfileCreate_RejectsBadSecretValues(t *testing.T) {
	cases := []struct {
		name string
		val  string
	}{
		{"whitespace only", "   "},
		{"newline only", "\n"},
		{"control char", "abc\x00def"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{"config", "profile", "create", "p1", "--api-secret", c.val, "--output-format", "json"}, &stdout, &stderr)
			if exit == 0 {
				t.Fatalf("profile create with bad secret should error; got exit 0 stdout=%s", stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "usage" {
				t.Errorf("code=%v, want usage", env["code"])
			}
		})
	}
}

// TestConfigProfileList_IsDefault_IsBool pins Round 8 MM: is_default
// used to be string "true"/"false" because the row was typed
// map[string]string. JSON consumers had to string-compare instead of
// branching on a bool. Now bool.
func TestConfigProfileList_IsDefault_IsBool(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xx"},
		"work":    {APISecret: "sec_work_xxxx"},
	})
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"config", "profile", "list", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	profiles, _ := data["profiles"].([]any)
	for _, p := range profiles {
		row, _ := p.(map[string]any)
		isDefault, ok := row["is_default"]
		if !ok {
			t.Fatalf("is_default field missing: %v", row)
		}
		if _, isBool := isDefault.(bool); !isBool {
			t.Errorf("is_default=%v (%T), want bool", isDefault, isDefault)
		}
	}
}
