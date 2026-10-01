package update_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/update"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"1.2.0", "1.3.0", true},
		{"1.2.0", "v1.2.1", true},
		{"v1.2.0", "2.0.0", true},
		{"1.2.0", "1.2.0", false},
		{"1.3.0", "1.2.9", false},
		{"1.10.0", "1.9.0", false}, // numeric, not lexical
		{"1.2.0-rc.1", "1.2.0", true},
		{"1.2.0", "1.3.0-rc.1", false}, // never nag towards a prerelease
		{"dev", "1.3.0", false},        // unversioned builds never nag
		{"1.2.0", "garbage", false},
		{"1.2.0", "", false},
	}
	for _, c := range cases {
		if got := update.IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestFetchLatest_ReadsTagFromGitHubReleaseShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept header = %q", r.Header.Get("Accept"))
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0","name":"v1.3.0","prerelease":false}`))
	}))
	defer srv.Close()

	got, err := update.FetchLatest(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if got != "1.3.0" {
		t.Errorf("FetchLatest = %q, want 1.3.0 (leading v stripped)", got)
	}
}

func TestFetchLatest_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden) // GitHub's unauthenticated rate limit
	}))
	defer srv.Close()

	if _, err := update.FetchLatest(context.Background(), srv.URL); err == nil {
		t.Fatal("expected an error for a 403")
	}
}

func TestFetchLatest_MissingTagIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	if _, err := update.FetchLatest(context.Background(), srv.URL); err == nil {
		t.Fatal("expected an error when tag_name is absent")
	}
}

func TestState_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "urlbox", "update-check.json")
	checked := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	if err := update.SaveState(path, update.State{CheckedAt: checked, Latest: "1.3.0"}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got := update.LoadState(path)
	if got.Latest != "1.3.0" || !got.CheckedAt.Equal(checked) {
		t.Errorf("LoadState = %+v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("state file mode = %o, want 600", perm)
	}
}

func TestLoadState_MissingOrCorruptIsZero(t *testing.T) {
	dir := t.TempDir()
	if s := update.LoadState(filepath.Join(dir, "absent.json")); !s.CheckedAt.IsZero() || s.Latest != "" {
		t.Errorf("missing file: got %+v", s)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := update.LoadState(bad); !s.CheckedAt.IsZero() || s.Latest != "" {
		t.Errorf("corrupt file: got %+v", s)
	}
}

func TestState_Stale(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if !(update.State{}).Stale(now) {
		t.Error("a never-checked state must be stale")
	}
	if (update.State{CheckedAt: now.Add(-23 * time.Hour)}).Stale(now) {
		t.Error("checked 23h ago must be fresh")
	}
	if !(update.State{CheckedAt: now.Add(-25 * time.Hour)}).Stale(now) {
		t.Error("checked 25h ago must be stale")
	}
	if !(update.State{CheckedAt: now.Add(time.Hour)}).Stale(now) {
		t.Error("a future timestamp (clock skew) must be treated as stale")
	}
}

func TestNotice(t *testing.T) {
	got := update.Notice("1.2.0", "1.3.0")
	want := "A new version of urlbox is available: 1.2.0 → 1.3.0. Run `urlbox upgrade` to update."
	if got != want {
		t.Errorf("Notice = %q\nwant     %q", got, want)
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{"1.2.0": true, "v1.2.0": true, "1.2.0-rc.1": true, "dev": false, "": false, "1.2": false} {
		if got := update.IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

// `make build` stamps `git describe` output: commits AFTER the tag, not a
// prerelease of it. Those builds must never be told to "upgrade" backwards.
func TestGitDescribeBuildsAreNotReleases(t *testing.T) {
	for _, v := range []string{"v1.2.0-1-gc9d96d6-dirty", "v1.2.0-14-g3a164cc", "v1.2.0-dirty"} {
		if update.IsRelease(v) {
			t.Errorf("IsRelease(%q) = true, want false", v)
		}
		if update.IsNewer(v, "1.2.0") || update.IsNewer(v, "1.3.0") {
			t.Errorf("IsNewer(%q, …) must be false for local builds", v)
		}
	}
}
