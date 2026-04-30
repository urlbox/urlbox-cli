package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
}

func TestPath_RespectsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	got := config.Path()
	want := "/tmp/xdg-test/urlbox/config.json"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestPath_DefaultsToHomeConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/fake-home")
	got := config.Path()
	want := "/tmp/fake-home/.config/urlbox/config.json"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestSave_Creates0600File(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIKey: "sec_test"}},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm=%v want 0600", info.Mode().Perm())
	}
}

func TestLoad_HandlesMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 0 || cfg.DefaultProfile != "" || cfg.LegacyAPIKey != "" {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestLoad_ReadsSavedConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	saved := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIKey: "sec_xyz"}},
	}
	if err := config.Save(saved); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Profiles["default"].APIKey; got != "sec_xyz" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPISecret_PrefersEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "env_secret")
	_ = config.Save(&config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APISecret: "file_secret"}},
	})
	if got := config.ResolveAPISecret(); got != "env_secret" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPISecret_FallsBackToFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	_ = config.Save(&config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APISecret: "file_secret"}},
	})
	if got := config.ResolveAPISecret(); got != "file_secret" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPISecret_EmptyWhenNoneSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	if got := config.ResolveAPISecret(); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestAPISecretSource_Env(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "env_secret")
	if src := config.APISecretSource(); src != "env" {
		t.Fatalf("got %q", src)
	}
}

func TestAPISecretSource_File(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	_ = config.Save(&config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APISecret: "file_secret"}},
	})
	if src := config.APISecretSource(); src != "file" {
		t.Fatalf("got %q", src)
	}
}

func TestAPISecretSource_None(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	if src := config.APISecretSource(); src != "none" {
		t.Fatalf("got %q", src)
	}
}

func TestConfig_LoadNewShape_MultiProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	must(t, os.MkdirAll(filepath.Join(dir, "urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "urlbox", "config.json"), []byte(`{
  "default_profile": "staging",
  "profiles": {
    "default": {"api_key": "pub_main", "api_secret": "sec_main"},
    "staging": {"api_key": "pub_stg", "api_secret": "sec_stg", "api_host": "https://api-staging.urlbox.com"}
  }
}`), 0o600))

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DefaultProfile != "staging" {
		t.Errorf("DefaultProfile = %q, want staging", c.DefaultProfile)
	}
	if len(c.Profiles) != 2 {
		t.Errorf("Profiles len = %d, want 2", len(c.Profiles))
	}
	if c.Profiles["staging"].APISecret != "sec_stg" {
		t.Errorf("staging.api_secret = %q", c.Profiles["staging"].APISecret)
	}
	if c.Profiles["staging"].APIHost != "https://api-staging.urlbox.com" {
		t.Errorf("staging.api_host = %q", c.Profiles["staging"].APIHost)
	}
}

func TestConfig_LoadLegacyShape_Migrates(t *testing.T) {
	// Phase 1's flat-shape file `{"api_key":"sec_xxx"}` always held the *secret*
	// (the field name was a mislabel — see CHANGELOG v0.6.0). Load migrates
	// the value into Profiles["default"].APISecret.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	must(t, os.MkdirAll(filepath.Join(dir, "urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "urlbox", "config.json"), []byte(`{"api_key":"sec_legacy"}`), 0o600))

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DefaultProfile != "default" {
		t.Errorf("DefaultProfile = %q, want default", c.DefaultProfile)
	}
	if got := c.Profiles["default"].APISecret; got != "sec_legacy" {
		t.Errorf("default.api_secret = %q, want sec_legacy (legacy migration)", got)
	}
	if got := c.Profiles["default"].APIKey; got != "" {
		t.Errorf("default.api_key = %q, want empty (Phase 1 mislabel: secret was in api_key field)", got)
	}
	if c.LegacyAPIKey != "sec_legacy" {
		t.Errorf("LegacyAPIKey = %q, want sec_legacy", c.LegacyAPIKey)
	}
}

func TestConfig_SaveAfterLegacyLoad_WritesNewShape(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	must(t, os.MkdirAll(filepath.Join(dir, "urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "urlbox", "config.json"), []byte(`{"api_key":"pub_legacy"}`), 0o600))

	c, err := config.Load()
	must(t, err)
	must(t, config.Save(c))

	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	must(t, err)
	got := string(b)
	if strings.Contains(got, `"api_key": "pub_legacy"`) && !strings.Contains(got, `"profiles"`) {
		t.Fatalf("file still in legacy shape after save:\n%s", got)
	}
	if !strings.Contains(got, `"default_profile": "default"`) {
		t.Errorf("expected default_profile in saved file:\n%s", got)
	}
	if !strings.Contains(got, `"profiles"`) {
		t.Errorf("expected profiles in saved file:\n%s", got)
	}
}

func TestProfile_IsEmpty(t *testing.T) {
	if !(config.Profile{}).IsEmpty() {
		t.Error("zero-value Profile should be IsEmpty")
	}
	if (config.Profile{APIKey: "x"}).IsEmpty() {
		t.Error("Profile with APIKey should not be IsEmpty")
	}
	if (config.Profile{APISecret: "x"}).IsEmpty() {
		t.Error("Profile with APISecret should not be IsEmpty")
	}
	if (config.Profile{APIHost: "x"}).IsEmpty() {
		t.Error("Profile with APIHost should not be IsEmpty")
	}
}

func TestConfig_Save_AtomicViaRename(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	c := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "pub_xxx", APISecret: "sec_xxx"},
		},
	}
	must(t, config.Save(c))

	matches, _ := filepath.Glob(filepath.Join(dir, "urlbox", "config.json.tmp*"))
	if len(matches) > 0 {
		t.Errorf("temp save file left behind: %v", matches)
	}

	info, err := os.Stat(filepath.Join(dir, "urlbox", "config.json"))
	must(t, err)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perm = %o, want 0600", perm)
	}
}
