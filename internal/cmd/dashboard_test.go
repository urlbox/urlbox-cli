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

func TestDashboard_OpensCorrectURL(t *testing.T) {
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
	if rec.calledWith != "https://urlbox.com/dashboard" {
		t.Errorf("opener called with %q, want %q", rec.calledWith, "https://urlbox.com/dashboard")
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["ok"] != true {
		t.Errorf("ok != true")
	}
	if env["command"] != "dashboard" {
		t.Errorf("command != 'dashboard': %v", env["command"])
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

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	if rec.calledWith != "" {
		t.Errorf("opener should not have been called in headless mode; got: %q", rec.calledWith)
	}
	if !strings.Contains(stderr.String(), "https://urlbox.com/dashboard") {
		t.Errorf("stderr should print URL in headless mode; got: %s", stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["ok"] != true {
		t.Errorf("ok != true")
	}
	summary, _ := env["summary"].(string)
	if !strings.Contains(summary, "no graphical session") {
		t.Errorf("summary should mention headless; got: %v", summary)
	}
}

func TestDashboard_OpenError_ServerExit(t *testing.T) {
	rec := &recordingOpener{returnErr: errors.New("xdg-open: command not found")}
	cmd.SetDashboardOpenerForTest(rec)
	t.Cleanup(cmd.ResetDashboardOpenerForTest)
	cmd.SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(cmd.ResetHeadlessDetectorForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"dashboard", "--output-format", "json"}, &stdout, &stderr)
	if exit != 10 {
		t.Fatalf("exit=%d, want 10 (server)", exit)
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "server" {
		t.Errorf("code != 'server': %v", env["code"])
	}
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, "https://urlbox.com/dashboard") {
		t.Errorf("hint should include the URL as a copy-paste fallback; got: %v", hint)
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
