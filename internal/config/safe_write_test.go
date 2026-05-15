// internal/config/safe_write_test.go — v1.0.4 Class 4.
//
// Pins the SafeWriteUserFile contract: Lstat refuses symlinks (no
// write-anywhere primitive), atomic rename (no half-written files
// visible mid-write), refuses to clobber existing content without
// Force=true (no silent destruction of user edits).
package config_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
)

func TestSafeWriteUserFile_CreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")

	err := config.SafeWriteUserFile(path, []byte("hello"), config.SafeWriteOptions{})
	if err != nil {
		t.Fatalf("write should succeed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
}

func TestSafeWriteUserFile_RefusesExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	if err := os.WriteFile(target, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported on this filesystem: %v", err)
	}

	err := config.SafeWriteUserFile(link, []byte("attacker"), config.SafeWriteOptions{Force: true})
	if err == nil {
		t.Fatal("expected refusal to follow symlink")
	}
	if !errors.Is(err, config.ErrSafeWriteSymlink) && !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected symlink-related error, got %v", err)
	}
	// Real target must be untouched.
	got, _ := os.ReadFile(target)
	if string(got) != "real" {
		t.Errorf("symlink target was overwritten: got %q", got)
	}
}

func TestSafeWriteUserFile_RefusesClobberWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")
	if err := os.WriteFile(path, []byte("user-edit"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := config.SafeWriteUserFile(path, []byte("clobber"), config.SafeWriteOptions{})
	if err == nil {
		t.Fatal("expected refusal to overwrite without Force")
	}
	if !errors.Is(err, config.ErrSafeWriteExists) && !strings.Contains(err.Error(), "exist") {
		t.Errorf("expected exists-related error, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "user-edit" {
		t.Errorf("file was overwritten without Force: got %q", got)
	}
}

func TestSafeWriteUserFile_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := config.SafeWriteUserFile(path, []byte("new"), config.SafeWriteOptions{Force: true})
	if err != nil {
		t.Fatalf("Force=true should succeed: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("expected 'new', got %q", got)
	}
}

func TestSafeWriteUserFile_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "deep", "file.md")

	err := config.SafeWriteUserFile(path, []byte("hi"), config.SafeWriteOptions{Mode: 0o600})
	if err != nil {
		t.Fatalf("expected mkdirs to happen: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %o, want 0o600", info.Mode().Perm())
	}
}

func TestSafeWriteUserFile_DefaultsToTight0600AndDir0700(t *testing.T) {
	// Skill files and config files are sensitive — defaults must be 0600/0700.
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.md")

	err := config.SafeWriteUserFile(path, []byte("hi"), config.SafeWriteOptions{})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("default file mode = %o, want 0o600", info.Mode().Perm())
	}
	dirInfo, _ := os.Stat(filepath.Dir(path))
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("default dir mode = %o, want 0o700", dirInfo.Mode().Perm())
	}
}

func TestSafeWriteUserFile_NoTempFileLeftBehind(t *testing.T) {
	// Atomic-rename contract: on success, no .tmp-* sidecar lingers.
	dir := t.TempDir()
	path := filepath.Join(dir, "x.md")

	if err := config.SafeWriteUserFile(path, []byte("hi"), config.SafeWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind after successful write: %s", e.Name())
		}
	}
}

func TestSafeWriteUserFile_NoPartialFileOnExistingClobberRefusal(t *testing.T) {
	// When SafeWriteUserFile refuses (existing file, no Force), the
	// original must be byte-identical afterwards (no partial truncation).
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")
	original := []byte("original content that must survive\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	_ = config.SafeWriteUserFile(path, []byte("attacker"), config.SafeWriteOptions{})

	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, original) {
		t.Errorf("original modified despite clobber refusal: got %q", got)
	}
}
