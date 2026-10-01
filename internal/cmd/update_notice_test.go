package cmd_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/update"
)

const noticeFragment = "A new version of urlbox is available"

// fakeRelease counts calls so tests can assert when the network is (not) hit.
type fakeRelease struct {
	latest string
	err    error
	calls  int
}

func (f *fakeRelease) fetch(context.Context) (string, error) {
	f.calls++
	return f.latest, f.err
}

// setupUpdateNotice isolates the config dir, forces an interactive stderr,
// clears the opt-out env vars (CI sets CI=true), and pins the running
// version to 1.2.0 with a fake release source.
func setupUpdateNotice(t *testing.T, rel *fakeRelease) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("CI", "")
	t.Setenv("URLBOX_NO_UPDATE_NOTIFIER", "")
	cmd.SetStderrTTYForTest(true)
	t.Cleanup(cmd.ResetStderrTTYForTest)
	cmd.SetUpdateCheckForTest("1.2.0", rel.fetch)
	t.Cleanup(cmd.ResetUpdateCheckForTest)
	return filepath.Join(dir, "urlbox", "update-check.json")
}

func runVersionText(t *testing.T) (stdout, stderr string, code int) {
	t.Helper()
	var out, errb bytes.Buffer
	code = cmd.Execute([]string{"version", "--output-format", "text"}, &out, &errb)
	return out.String(), errb.String(), code
}

func TestUpdateNotice_StaleCache_FetchesAndPrintsToStderr(t *testing.T) {
	rel := &fakeRelease{latest: "1.3.0"}
	statePath := setupUpdateNotice(t, rel)

	stdout, stderr, code := runVersionText(t)

	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if rel.calls != 1 {
		t.Errorf("fetch calls = %d, want 1", rel.calls)
	}
	if !strings.Contains(stderr, "1.2.0 → 1.3.0") || !strings.Contains(stderr, "urlbox upgrade") {
		t.Errorf("stderr missing notice: %q", stderr)
	}
	if strings.Contains(stdout, noticeFragment) {
		t.Errorf("notice leaked onto stdout: %q", stdout)
	}
	if s := update.LoadState(statePath); s.Latest != "1.3.0" || s.CheckedAt.IsZero() {
		t.Errorf("cache not written: %+v", s)
	}
}

func TestUpdateNotice_FreshCache_NoFetchStillNotifies(t *testing.T) {
	rel := &fakeRelease{latest: "9.9.9"}
	statePath := setupUpdateNotice(t, rel)
	if err := update.SaveState(statePath, update.State{CheckedAt: time.Now().Add(-time.Hour), Latest: "1.3.0"}); err != nil {
		t.Fatal(err)
	}

	_, stderr, _ := runVersionText(t)

	if rel.calls != 0 {
		t.Errorf("fresh cache must not hit the network; calls = %d", rel.calls)
	}
	if !strings.Contains(stderr, "1.2.0 → 1.3.0") {
		t.Errorf("expected notice from cache, got %q", stderr)
	}
}

func TestUpdateNotice_UpToDate_Silent(t *testing.T) {
	rel := &fakeRelease{latest: "1.2.0"}
	setupUpdateNotice(t, rel)

	_, stderr, _ := runVersionText(t)

	if strings.Contains(stderr, noticeFragment) {
		t.Errorf("no notice expected when current: %q", stderr)
	}
}

func TestUpdateNotice_FetchFails_SilentAndBacksOff(t *testing.T) {
	rel := &fakeRelease{err: errors.New("offline")}
	statePath := setupUpdateNotice(t, rel)

	_, stderr, code := runVersionText(t)

	if code != 0 {
		t.Fatalf("a failed check must never fail the command; exit %d", code)
	}
	if strings.Contains(stderr, noticeFragment) || strings.Contains(stderr, "offline") {
		t.Errorf("failed check must be silent: %q", stderr)
	}
	// The attempt is recorded so an offline machine doesn't retry every run.
	if s := update.LoadState(statePath); s.CheckedAt.IsZero() {
		t.Error("failed check should still record CheckedAt")
	}
}

func TestUpdateNotice_Gates(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		setup func(t *testing.T)
	}{
		{"json output", []string{"version", "--output-format", "json"}, nil},
		{"quiet output", []string{"version", "--output-format", "quiet"}, nil},
		{"jq", []string{"version", "--output-format", "text", "--jq", ".data"}, nil},
		{"stdout piped, no explicit format", []string{"version"}, nil},
		{"stderr not a tty", []string{"version", "--output-format", "text"}, func(*testing.T) { cmd.SetStderrTTYForTest(false) }},
		{"opt-out env", []string{"version", "--output-format", "text"}, func(t *testing.T) { t.Setenv("URLBOX_NO_UPDATE_NOTIFIER", "1") }},
		{"CI env", []string{"version", "--output-format", "text"}, func(t *testing.T) { t.Setenv("CI", "true") }},
		{"dev build", []string{"version", "--output-format", "text"}, func(*testing.T) { cmd.SetUpdateCheckForTest("dev", (&fakeRelease{latest: "1.3.0"}).fetch) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rel := &fakeRelease{latest: "1.3.0"}
			setupUpdateNotice(t, rel)
			if c.setup != nil {
				c.setup(t)
			}
			var out, errb bytes.Buffer
			cmd.Execute(c.args, &out, &errb)
			if rel.calls != 0 {
				t.Errorf("gated run must not hit the network; calls = %d", rel.calls)
			}
			if strings.Contains(out.String()+errb.String(), noticeFragment) {
				t.Errorf("gated run printed the notice: stdout=%q stderr=%q", out.String(), errb.String())
			}
		})
	}
}

func TestUpdateNotice_PrintedAfterFailedCommand(t *testing.T) {
	rel := &fakeRelease{latest: "1.3.0"}
	setupUpdateNotice(t, rel)

	var out, errb bytes.Buffer
	code := cmd.Execute([]string{"config", "get", "no_such_key", "--output-format", "text"}, &out, &errb)

	if code == 0 {
		t.Fatal("expected the command itself to fail")
	}
	stderr := errb.String()
	errAt := strings.Index(stderr, "Error")
	noticeAt := strings.Index(stderr, noticeFragment)
	if errAt < 0 || noticeAt < 0 || noticeAt < errAt {
		t.Errorf("notice should follow the error line: %q", stderr)
	}
}
