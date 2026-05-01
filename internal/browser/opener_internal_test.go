package browser

import "testing"

// White-box: pin the Windows dispatch on non-Windows hosts so the test
// runs on every platform.
func TestOSOpener_WindowsArgs_HasEmptyTitle(t *testing.T) {
	// `cmd /c start "" "<url>"` — empty title is required so a URL with
	// spaces isn't misinterpreted as the window title.
	var o OSOpener
	cmd, args := o.commandForGOOS("windows", "https://example.com/with spaces")
	if cmd != "cmd" {
		t.Errorf("cmd=%q, want cmd", cmd)
	}
	if len(args) != 4 || args[0] != "/c" || args[1] != "start" || args[2] != "" {
		t.Errorf("windows args=%v, want [/c start \"\" <url>]", args)
	}
}

func TestOSOpener_LinuxFallback_NonStandardGOOS(t *testing.T) {
	// Anything that's not darwin/windows falls through to xdg-open.
	var o OSOpener
	cmd, args := o.commandForGOOS("freebsd", "https://example.com")
	if cmd != "xdg-open" {
		t.Errorf("freebsd cmd=%q, want xdg-open", cmd)
	}
	if len(args) != 1 || args[0] != "https://example.com" {
		t.Errorf("freebsd args=%v", args)
	}
}
