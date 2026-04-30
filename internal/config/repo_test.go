package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
)

func TestRepoOverlay_NotFound_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	overlay, err := config.LoadRepoOverlay(dir, dir)
	if err != nil {
		t.Fatalf("LoadRepoOverlay: %v", err)
	}
	if overlay != nil {
		t.Fatalf("expected nil, got %+v", overlay)
	}
}

func TestRepoOverlay_FoundInCWD(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(root, ".urlbox", "config.json"),
		[]byte(`{"profile":"staging","api_key":"pub_repo"}`), 0o600))

	overlay, err := config.LoadRepoOverlay(root, "/")
	must(t, err)
	if overlay == nil {
		t.Fatal("expected overlay, got nil")
	}
	if overlay.Profile != "staging" {
		t.Errorf("Profile = %q", overlay.Profile)
	}
	if overlay.APIKey != "pub_repo" {
		t.Errorf("APIKey = %q", overlay.APIKey)
	}
	if overlay.Path != filepath.Join(root, ".urlbox", "config.json") {
		t.Errorf("Path = %q", overlay.Path)
	}
}

func TestRepoOverlay_FoundInAncestor(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "a", "b", "c")
	must(t, os.MkdirAll(cwd, 0o755))
	must(t, os.MkdirAll(filepath.Join(root, ".urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(root, ".urlbox", "config.json"),
		[]byte(`{"api_secret":"sec_repo"}`), 0o600))

	overlay, err := config.LoadRepoOverlay(cwd, "/")
	must(t, err)
	if overlay == nil {
		t.Fatal("expected overlay, got nil")
	}
	if overlay.APISecret != "sec_repo" {
		t.Errorf("APISecret = %q", overlay.APISecret)
	}
}

func TestRepoOverlay_StopsAtBoundary(t *testing.T) {
	root := t.TempDir()
	boundary := filepath.Join(root, "home")
	cwd := filepath.Join(boundary, "project")
	must(t, os.MkdirAll(cwd, 0o755))
	must(t, os.MkdirAll(filepath.Join(root, ".urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(root, ".urlbox", "config.json"),
		[]byte(`{"api_key":"pub_outside"}`), 0o600))

	overlay, err := config.LoadRepoOverlay(cwd, boundary)
	must(t, err)
	if overlay != nil {
		t.Fatalf("expected nil (overlay outside boundary), got %+v", overlay)
	}
}

func TestRepoOverlay_MalformedJSON_ReturnsError(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, ".urlbox"), 0o700))
	must(t, os.WriteFile(filepath.Join(root, ".urlbox", "config.json"),
		[]byte(`{not json`), 0o600))

	_, err := config.LoadRepoOverlay(root, "/")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}
