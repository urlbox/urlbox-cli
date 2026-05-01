// Package browser opens URLs in the user's default browser. The Opener
// interface lets tests inject a fake; OSOpener is the production
// implementation that shells out to the platform's URL handler.
package browser

import (
	"os/exec"
	"runtime"
)

// Opener opens a URL in the default browser. Mockable for tests; the
// render command's --open flag invokes the active Opener.
type Opener interface {
	Open(url string) error
}

// OSOpener is the production Opener that shells out to the platform's
// URL handler: `open` (macOS), `xdg-open` (Linux + others), or
// `cmd /c start "" <url>` (Windows). Failures are returned as-is; the
// caller decides whether to surface or swallow them.
type OSOpener struct{}

// NewOSOpener constructs an OSOpener.
func NewOSOpener() *OSOpener { return &OSOpener{} }

// CommandFor returns the command + args that will be invoked for the
// given URL on the current GOOS. Exposed so tests can pin the dispatch
// table without actually executing anything.
func (o *OSOpener) CommandFor(url string) (cmd string, args []string) {
	return o.commandForGOOS(runtime.GOOS, url)
}

// commandForGOOS resolves the platform-specific command. Split out from
// CommandFor so tests can pin the Windows dispatch on non-Windows hosts.
func (*OSOpener) commandForGOOS(goos, url string) (cmd string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		// `cmd /c start "" "<url>"` — empty title is required so a URL
		// with spaces isn't misinterpreted as the window title.
		return "cmd", []string{"/c", "start", "", url}
	default: // linux, freebsd, openbsd, netbsd, ...
		return "xdg-open", []string{url}
	}
}

// Open launches the default browser at the given URL. Returns whatever
// error the OS handler does; the render command treats opener failures
// as non-fatal (the rendered URL is still in the envelope).
//
//nolint:gosec,noctx // best-effort browser launch — user-supplied URL is the whole point of the command, and there's no caller-supplied context to thread (failures are silently ignored upstream).
func (o *OSOpener) Open(url string) error {
	name, args := o.CommandFor(url)
	return exec.Command(name, args...).Start()
}

// NoopOpener does nothing. Useful as a default in headless environments
// or when an upstream wants to prevent any browser launch entirely.
type NoopOpener struct{}

// Open returns nil without doing anything.
func (NoopOpener) Open(_ string) error { return nil }
