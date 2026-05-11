package cmd

// White-box tests for resolveOutputPath. These are small, dense, and
// security-relevant — pin every rejection case so a regression in path
// canonicalization is caught immediately.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
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

// TestResolveOutputPath_RejectsOutsideCWD_HintMentionsCdWorkaround pins
// the rejection hint includes the `cd <dir> && urlbox render ...` escape
// hatch. Round 1 UX I4 — agents bouncing off /tmp/foo.png deserved to be
// told the workaround inline, not just "pass a path under CWD".
func TestResolveOutputPath_RejectsOutsideCWD_HintMentionsCdWorkaround(t *testing.T) {
	cwd := t.TempDir()
	_, err := resolveOutputPath("/tmp/foo.png", cwd)
	if err == nil {
		t.Fatal("expected error for /tmp/foo.png outside CWD")
	}
	if !strings.Contains(err.Hint, "cd ") {
		t.Errorf("hint should mention `cd` workaround; got %q", err.Hint)
	}
	if !strings.Contains(err.Hint, "urlbox render") {
		t.Errorf("hint should show the workaround command; got %q", err.Hint)
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

// canonical returns the EvalSymlinks form of dir. resolveOutputPath returns
// canonical paths (post-EvalSymlinks), so test assertions compare against
// canonical too — otherwise macOS `/var` vs `/private/var` makes them flake.
func canonical(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return r
}

func TestResolveOutputPath_AcceptsSimpleFilename(t *testing.T) {
	cwd := t.TempDir()
	got, err := resolveOutputPath("out.png", cwd)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := filepath.Join(canonical(t, cwd), "out.png")
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
	want := filepath.Join(canonical(t, cwd), "renders", "out.png")
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
	want := filepath.Join(canonical(t, cwd), "out.png")
	if got != want {
		t.Errorf("got=%q, want %q", got, want)
	}
}

func TestResolveOutputPath_AcceptsDotSlashFile(t *testing.T) {
	cwd := t.TempDir()
	got, err := resolveOutputPath("./out.png", cwd)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	want := filepath.Join(canonical(t, cwd), "out.png")
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

func TestResolveOutputPath_RejectsSymlinkParentEscape(t *testing.T) {
	// An attacker who can write into CWD could plant a symlink that
	// redirects --output writes to arbitrary paths (e.g. /etc/passwd).
	// EvalSymlinks must catch this.
	cwd := t.TempDir()
	target := t.TempDir() // Some "outside" dir we want to protect.
	link := filepath.Join(cwd, "evil-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}
	_, err := resolveOutputPath("evil-link/foo.png", cwd)
	if err == nil {
		t.Fatal("expected rejection: writing through a symlink that points outside CWD")
	}
	if string(err.Code) != "validation" {
		t.Errorf("code=%v, want validation", err.Code)
	}
}

func TestResolveOutputPath_AcceptsSymlinkInsideCWD(t *testing.T) {
	// Symlinks that stay under CWD are fine — only escapes are rejected.
	cwd := t.TempDir()
	innerDir := filepath.Join(cwd, "real-dir")
	if err := os.Mkdir(innerDir, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cwd, "alias")
	if err := os.Symlink(innerDir, link); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}
	_, err := resolveOutputPath("alias/foo.png", cwd)
	if err != nil {
		t.Errorf("symlink to a dir under CWD should be accepted, got %v", err)
	}
}

func TestResolveOutputPath_BareTilde_ExpandsToHome(t *testing.T) {
	// A bare `~` should expand to $HOME (then sandbox-check against CWD),
	// not silently become a literal filename "~". This is the same
	// behavior as `~/path` — bare ~ is just the no-suffix case.
	cwd := t.TempDir()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home dir on this machine")
	}
	if strings.HasPrefix(cwd, home) {
		t.Skip("CWD is under HOME; rejection wouldn't trigger")
	}
	_, perr := resolveOutputPath("~", cwd)
	if perr == nil {
		t.Fatal("expected rejection: bare ~ resolves to $HOME, which is outside CWD")
	}
	if string(perr.Code) != "validation" {
		t.Errorf("code=%v, want validation", perr.Code)
	}
}

func TestDownloadTo_RejectsLeafSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "real-target.png")
	if err := os.WriteFile(target, []byte("untouchable"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "out.png")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("malicious-content"))
	}))
	defer srv.Close()

	cliErr := downloadTo(context.Background(), srv.URL, link)
	if cliErr == nil {
		t.Fatal("expected error rejecting leaf symlink, got nil")
	}
	if cliErr.Code != output.ErrValidation {
		t.Errorf("code=%v, want validation", cliErr.Code)
	}
	body, _ := os.ReadFile(target)
	if string(body) != "untouchable" {
		t.Errorf("symlink target was overwritten: %q", string(body))
	}
}

// Regression guard (v1.0.2): --output "-" must be rejected explicitly. The
// shell convention is that "-" means stdout streaming; without an explicit
// rejection the path falls through and a literal file named "-" is created
// in the CWD — surprising and easy to miss in directory listings.
func TestResolveOutputPath_RejectsDashAsStdout(t *testing.T) {
	cwd := t.TempDir()
	_, err := resolveOutputPath("-", cwd)
	if err == nil {
		t.Fatal("expected error rejecting '-'")
	}
	if err.Code != output.ErrValidation {
		t.Errorf("code=%v, want validation", err.Code)
	}
	if !strings.Contains(err.Message, "-") {
		t.Errorf("error should mention the dash; got %q", err.Message)
	}
}

func TestDownloadTo_AcceptsExistingRegularFile(t *testing.T) {
	tmp := t.TempDir()
	dst := filepath.Join(tmp, "out.png")
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("new-content"))
	}))
	defer srv.Close()

	if cliErr := downloadTo(context.Background(), srv.URL, dst); cliErr != nil {
		t.Fatalf("unexpected error: %v", cliErr)
	}
	body, _ := os.ReadFile(dst)
	if string(body) != "new-content" {
		t.Errorf("file not overwritten: %q", string(body))
	}
}
