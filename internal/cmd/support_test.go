package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type fakeSupportOpener struct{ opened []string }

func (f *fakeSupportOpener) Open(url string) error {
	f.opened = append(f.opened, url)
	return nil
}

func TestSupport_TextMode_OpensContactPage(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	SetHeadlessDetectorForTest(func() bool { return false })
	t.Cleanup(ResetHeadlessDetectorForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if len(fake.opened) != 1 || fake.opened[0] != "https://urlbox.com/contact" {
		t.Fatalf("opened: %v", fake.opened)
	}
}

func TestSupport_JSONMode_NoBrowserEmitsEnvelope(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(fake.opened) != 0 {
		t.Fatalf("json mode must not launch a browser: %v", fake.opened)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if env["command"] != "support" || data["url"] != "https://urlbox.com/contact" {
		t.Fatalf("envelope: %v", env)
	}
}

func TestSupport_Headless_PrintsURLToStderr(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	SetHeadlessDetectorForTest(func() bool { return true })
	t.Cleanup(ResetHeadlessDetectorForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(fake.opened) != 0 {
		t.Fatalf("headless must not launch: %v", fake.opened)
	}
	if !strings.Contains(stderr.String(), "https://urlbox.com/contact") {
		t.Fatalf("stderr must carry the URL: %q", stderr.String())
	}
}

func TestSupport_QuietMode_EmitsBareURL(t *testing.T) {
	fake := &fakeSupportOpener{}
	SetSupportOpenerForTest(fake)
	t.Cleanup(ResetSupportOpenerForTest)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"support", "--output-format", "quiet"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(stdout.String()) != "https://urlbox.com/contact" {
		t.Fatalf("quiet stdout=%q", stdout.String())
	}
	if len(fake.opened) != 0 {
		t.Fatalf("quiet mode must not launch: %v", fake.opened)
	}
}
