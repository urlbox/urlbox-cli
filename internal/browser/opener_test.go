package browser_test

import (
	"runtime"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/browser"
)

func TestOSOpener_PicksCorrectBinaryByGOOS(t *testing.T) {
	o := browser.NewOSOpener()
	cmd, args := o.CommandFor("https://example.com")
	switch runtime.GOOS {
	case "darwin":
		if cmd != "open" {
			t.Errorf("darwin cmd=%q, want open", cmd)
		}
	case "linux":
		if cmd != "xdg-open" {
			t.Errorf("linux cmd=%q, want xdg-open", cmd)
		}
	case "windows":
		if cmd != "cmd" {
			t.Errorf("windows cmd=%q, want cmd", cmd)
		}
	}
	if len(args) == 0 || args[len(args)-1] != "https://example.com" {
		t.Errorf("args=%v, expected URL as last arg", args)
	}
}

func TestNoopOpener_DoesNothing(t *testing.T) {
	var n browser.NoopOpener
	if err := n.Open("https://example.com"); err != nil {
		t.Errorf("NoopOpener.Open() = %v, want nil", err)
	}
}

// ─── Class 2.2 (v1.0.4) ─── scheme check before OS exec ────────────
//
// Invariant: any URL handed to the OS URL handler is scheme-checked
// (http or https only) before exec.Command. Pre-v1.0.4 Open() forwarded
// any string. With a hostile api_host returning an attacker-controlled
// renderUrl, the Windows `cmd /c start "" <url>` path would launch
// UNC paths and file:// schemes — turning --open into a remote-launch
// primitive. Defense-in-depth: even if Class 1 (validation) closes
// every overlay/profile path today, this stops a future regression
// from re-arming the primitive.

func TestOSOpener_Open_RejectsNonHTTPSchemes(t *testing.T) {
	o := browser.NewOSOpener()
	for _, target := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ftp://example.com/x",
		"data:text/html,<script>alert(1)</script>",
		"chrome://settings",
		`\\server\share\evil.exe`, // UNC path (Windows-relevant)
		"",
		"not a url at all",
	} {
		if err := o.Open(target); err == nil {
			t.Errorf("Open(%q) should refuse, got nil", target)
		}
	}
}

func TestOSOpener_CommandFor_RefusesNonHTTPSchemes(t *testing.T) {
	// CommandFor is the test-friendly surface — it should refuse to even
	// produce a command for a non-http(s) URL so callers that go through
	// CommandFor (the pinned dispatch table in tests) match the runtime
	// behaviour of Open.
	o := browser.NewOSOpener()
	for _, target := range []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"ftp://x.example",
		"",
	} {
		cmd, args := o.CommandFor(target)
		if cmd != "" || args != nil {
			t.Errorf("CommandFor(%q) should return empty, got cmd=%q args=%v", target, cmd, args)
		}
	}
}

func TestOSOpener_Open_AllowsHTTPAndHTTPS(t *testing.T) {
	// Allowed schemes still produce a command (we don't actually exec —
	// CommandFor is the observable here).
	o := browser.NewOSOpener()
	for _, target := range []string{
		"https://example.com",
		"http://localhost:3000",
		"https://example.com/path?q=1#frag",
	} {
		cmd, _ := o.CommandFor(target)
		if cmd == "" {
			t.Errorf("CommandFor(%q) should produce a command, got empty", target)
		}
	}
}
