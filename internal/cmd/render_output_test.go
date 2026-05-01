package cmd

// White-box tests for resolveOutputPath. These are small, dense, and
// security-relevant — pin every rejection case so a regression in path
// canonicalization is caught immediately.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveOutputPath_RejectsEmpty(t *testing.T) {
	_, err := resolveOutputPath("", "/cwd")
	if err == nil {
		t.Fatal("expected error")
	}
	if string(err.Code) != "validation" {
		t.Errorf("code=%v, want validation", err.Code)
	}
}

func TestResolveOutputPath_RejectsParentEscape(t *testing.T) {
	cwd := t.TempDir()
	_, err := resolveOutputPath("../escape.png", cwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if string(err.Code) != "validation" {
		t.Errorf("code=%v, want validation", err.Code)
	}
	if !strings.Contains(err.Message, "outside") && !strings.Contains(err.Message, "escapes") {
		t.Errorf("message=%q should mention outside/escapes", err.Message)
	}
}

func TestResolveOutputPath_RejectsAbsoluteOutsideCWD(t *testing.T) {
	cwd := t.TempDir()
	_, err := resolveOutputPath("/etc/passwd", cwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if string(err.Code) != "validation" {
		t.Errorf("code=%v, want validation", err.Code)
	}
}

func TestResolveOutputPath_RejectsHomeOutsideCWD(t *testing.T) {
	cwd := t.TempDir()
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home dir on this machine")
	}
	if strings.HasPrefix(cwd, home) {
		t.Skip("CWD is under HOME; rejection wouldn't trigger")
	}
	_, err := resolveOutputPath("~/secrets.png", cwd)
	if err == nil {
		t.Fatal("expected error")
	}
	if string(err.Code) != "validation" {
		t.Errorf("code=%v, want validation", err.Code)
	}
}

func TestResolveOutputPath_RejectsNullByte(t *testing.T) {
	_, err := resolveOutputPath("file\x00.png", t.TempDir())
	if err == nil {
		t.Fatal("expected error")
	}
	if string(err.Code) != "validation" {
		t.Errorf("code=%v, want validation", err.Code)
	}
}

func TestResolveOutputPath_AcceptsSimpleFilename(t *testing.T) {
	cwd := t.TempDir()
	got, err := resolveOutputPath("out.png", cwd)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := filepath.Join(cwd, "out.png")
	if got != want {
		t.Errorf("got=%q, want %q", got, want)
	}
}

func TestResolveOutputPath_AcceptsRelativeSubdir(t *testing.T) {
	cwd := t.TempDir()
	got, err := resolveOutputPath("renders/out.png", cwd)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := filepath.Join(cwd, "renders", "out.png")
	if got != want {
		t.Errorf("got=%q, want %q", got, want)
	}
}

func TestResolveOutputPath_AcceptsAbsoluteUnderCWD(t *testing.T) {
	cwd := t.TempDir()
	inside := filepath.Join(cwd, "out.png")
	got, err := resolveOutputPath(inside, cwd)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got != inside {
		t.Errorf("got=%q, want %q", got, inside)
	}
}

func TestResolveOutputPath_AcceptsDotSlashFile(t *testing.T) {
	cwd := t.TempDir()
	got, err := resolveOutputPath("./out.png", cwd)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := filepath.Join(cwd, "out.png")
	if got != want {
		t.Errorf("got=%q, want %q (./ prefix should normalize away)", got, want)
	}
}

func TestResolveOutputPath_RejectsTrailingSlash(t *testing.T) {
	// A trailing slash means "this is a directory" — but --output must
	// resolve to a file path. Reject so we don't accidentally treat a
	// directory name as a file.
	_, err := resolveOutputPath("output/", t.TempDir())
	if err == nil {
		t.Fatal("expected error for trailing slash")
	}
}
