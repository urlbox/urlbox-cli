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
