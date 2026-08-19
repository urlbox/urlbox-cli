package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileSessionFieldsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	err := Save(&Config{
		DefaultProfile: "default",
		Profiles: map[string]Profile{"default": {
			APISecret:     "sk_live_1234567890",
			SessionToken:  "sess_tok_abcdef123456",
			ActiveOrg:     "org_01hxyz",
			ActiveProject: "proj_01habc",
		}},
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := loaded.Profiles["default"]
	if p.SessionToken != "sess_tok_abcdef123456" {
		t.Fatalf("session token dropped on roundtrip: %+v", p)
	}
	if p.ActiveOrg != "org_01hxyz" || p.ActiveProject != "proj_01habc" {
		t.Fatalf("active org/project dropped: %+v", p)
	}
	info, err := os.Stat(Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, want 0600", info.Mode().Perm())
	}
	if filepath.Dir(Path()) == "" {
		t.Fatal("empty config dir")
	}
}

func TestProfileIsEmptyCountsSessionFields(t *testing.T) {
	if (Profile{SessionToken: "tok"}).IsEmpty() {
		t.Fatal("profile with only a session token must not be IsEmpty")
	}
	if !(Profile{}).IsEmpty() {
		t.Fatal("zero profile must be IsEmpty")
	}
}

func TestProfileNameSelectionChain(t *testing.T) {
	cfg := &Config{DefaultProfile: "team", Profiles: map[string]Profile{"team": {}}}
	if got := ProfileName("flagged", "enved", &RepoOverlay{Profile: "repo"}, cfg); got != "flagged" {
		t.Fatalf("flag must win, got %q", got)
	}
	if got := ProfileName("", "enved", &RepoOverlay{Profile: "repo"}, cfg); got != "repo" {
		t.Fatalf("repo overlay must beat env, got %q", got)
	}
	if got := ProfileName("", "enved", nil, cfg); got != "enved" {
		t.Fatalf("env must beat default_profile, got %q", got)
	}
	if got := ProfileName("", "", nil, cfg); got != "team" {
		t.Fatalf("default_profile must beat literal default, got %q", got)
	}
	if got := ProfileName("", "", nil, &Config{}); got != "default" {
		t.Fatalf("fallback must be default, got %q", got)
	}
}
