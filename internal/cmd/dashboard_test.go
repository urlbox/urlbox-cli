package cmd_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

type recordingOpener struct {
	calledWith string
	returnErr  error
}

func (r *recordingOpener) Open(url string) error {
	r.calledWith = url
	return r.returnErr
}

// TestDashboard_OpensBrowser_WhenTextFormat pins that in the default
// text/TTY case, the browser opener is invoked. The "real user typing
// urlbox dashboard at a terminal" flow.
func TestDashboard_OpensBrowser_WhenTextFormat(t *testing.T) {
	rec := &recordingOpener{}
	cmd.SetDashboardOpenerForTest(rec)
	t.Cleanup(cmd.ResetDashboardOpenerForTest)
	cmd.SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(cmd.ResetHeadlessDetectorForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "text"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if rec.calledWith != "https://urlbox.com/dashboard" {
		t.Errorf("opener called with %q, want %q", rec.calledWith, "https://urlbox.com/dashboard")
	}
}

// TestDashboard_JSONFormat_NoBrowserSideEffect pins Round 8 MM /
// Adv-4 M4: when the user asks for json (or quiet), they're scripting
// around the URL — don't launch a browser tab as a side effect. The
// envelope still carries the URL.
func TestDashboard_JSONFormat_NoBrowserSideEffect(t *testing.T) {
	rec := &recordingOpener{}
	cmd.SetDashboardOpenerForTest(rec)
	t.Cleanup(cmd.ResetDashboardOpenerForTest)
	cmd.SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(cmd.ResetHeadlessDetectorForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if rec.calledWith != "" {
		t.Errorf("browser opener should NOT be called in json mode; got %q", rec.calledWith)
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["ok"] != true {
		t.Errorf("ok != true")
	}
	data := env["data"].(map[string]any)
	if data["url"] != "https://urlbox.com/dashboard" {
		t.Errorf("data.url mismatch: %v", data["url"])
	}
}

func TestDashboard_HeadlessFallback_PrintsURL(t *testing.T) {
	cmd.SetHeadlessDetectorForTest(func() bool { return true })
	t.Cleanup(cmd.ResetHeadlessDetectorForTest)
	rec := &recordingOpener{returnErr: errors.New("should not be called")}
	cmd.SetDashboardOpenerForTest(rec)
	t.Cleanup(cmd.ResetDashboardOpenerForTest)

	// Use text format so we hit the headless code path (json path now
	// short-circuits before reaching the headless check — Round 8 MM).
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "text"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if rec.calledWith != "" {
		t.Errorf("opener should not have been called in headless mode; got: %q", rec.calledWith)
	}
	if !strings.Contains(stderr.String(), "https://urlbox.com/dashboard") {
		t.Errorf("stderr should print URL in headless mode; got: %s", stderr.String())
	}
}

func TestDashboard_OpenError_ServerExit(t *testing.T) {
	rec := &recordingOpener{returnErr: errors.New("xdg-open: command not found")}
	cmd.SetDashboardOpenerForTest(rec)
	t.Cleanup(cmd.ResetDashboardOpenerForTest)
	cmd.SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(cmd.ResetHeadlessDetectorForTest)

	// Use text format so we exercise the actual opener path — json
	// mode never calls the opener now (Round 8 MM).
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "text"}, &stdout, &stderr)
	if exit != 10 {
		t.Fatalf("exit=%d, want 10 (server)", exit)
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "https://urlbox.com/dashboard") {
		t.Errorf("error output should include the URL as a copy-paste fallback; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestDashboard_QuietOutput(t *testing.T) {
	rec := &recordingOpener{}
	cmd.SetDashboardOpenerForTest(rec)
	t.Cleanup(cmd.ResetDashboardOpenerForTest)
	cmd.SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(cmd.ResetHeadlessDetectorForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	got := strings.TrimSpace(stdout.String())
	if got != "https://urlbox.com/dashboard" {
		t.Errorf("quiet stdout = %q, want exact URL", got)
	}
}
