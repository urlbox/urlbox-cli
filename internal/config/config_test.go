package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
)

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
	if err := config.Save(&config.Config{APIKey: "sec_test"}); err != nil {
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
	if cfg.APIKey != "" {
		t.Fatalf("expected empty APIKey, got %q", cfg.APIKey)
	}
}

func TestLoad_ReadsSavedConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(&config.Config{APIKey: "sec_xyz"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "sec_xyz" {
		t.Fatalf("got %q", cfg.APIKey)
	}
}

func TestResolveAPIKey_PrefersEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "env_key")
	_ = config.Save(&config.Config{APIKey: "file_key"})
	if got := config.ResolveAPIKey(); got != "env_key" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPIKey_FallsBackToFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	_ = config.Save(&config.Config{APIKey: "file_key"})
	if got := config.ResolveAPIKey(); got != "file_key" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPIKey_EmptyWhenNoneSet(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	if got := config.ResolveAPIKey(); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveAPIKeySource_Env(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "env_key")
	if src := config.APIKeySource(); src != "env" {
		t.Fatalf("got %q", src)
	}
}

func TestResolveAPIKeySource_File(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	_ = config.Save(&config.Config{APIKey: "file_key"})
	if src := config.APIKeySource(); src != "file" {
		t.Fatalf("got %q", src)
	}
}

func TestResolveAPIKeySource_None(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	if src := config.APIKeySource(); src != "none" {
		t.Fatalf("got %q", src)
	}
}
