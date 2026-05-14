// Package browser opens URLs in the user's default browser. The Opener
// interface lets tests inject a fake; OSOpener is the production
// implementation that shells out to the platform's URL handler.
package browser

import (
	"errors"
	"net/url"
	"os/exec"
	"runtime"
)

// ErrUnopenableURL is returned by OSOpener.Open when the URL's scheme is
// neither http nor https. The render command treats opener errors as
// non-fatal, so this surfaces as a debug-level signal rather than a
// hard fail — but the URL is NOT handed to the OS.
var ErrUnopenableURL = errors.New("refused to open URL: scheme must be http or https")

// isOpenableURL returns true if raw is safe to pass to the OS URL
// handler. Only http and https schemes are accepted; anything else
// (file, javascript, data, ftp, chrome, UNC paths) is refused — these
// typically signal an attacker-controlled URL slipped through (e.g.
// via a hostile api_host that returned an arbitrary renderUrl) and
// the OS handler would happily launch executables for some of them.
//
// v1.0.4 Class 2.2: pre-1.0.4 Open() forwarded any string, turning
// --open into a remote-launch primitive on Windows (cmd /c start
// happily resolves UNC paths and file: schemes to executables).
func isOpenableURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

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
//
// Returns ("", nil) if the URL's scheme is not http or https — the
// scheme gate is enforced here so callers that go through CommandFor
// (tests pinning the dispatch table) see the same refusal as Open.
func (o *OSOpener) CommandFor(rawURL string) (cmd string, args []string) {
	if !isOpenableURL(rawURL) {
		return "", nil
	}
	return o.commandForGOOS(runtime.GOOS, rawURL)
}

// commandForGOOS resolves the platform-specific command. Split out from
// CommandFor so tests can pin the Windows dispatch on non-Windows hosts.
func (*OSOpener) commandForGOOS(goos, rawURL string) (cmd string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{rawURL}
	case "windows":
		// `cmd /c start "" "<url>"` — empty title is required so a URL
		// with spaces isn't misinterpreted as the window title.
		return "cmd", []string{"/c", "start", "", rawURL}
	default: // linux, freebsd, openbsd, netbsd, ...
		return "xdg-open", []string{rawURL}
	}
}

// Open launches the default browser at the given URL. Returns
// ErrUnopenableURL when the scheme is not http or https — the render
// command treats opener failures as non-fatal (the rendered URL is
// still in the envelope), so this surfaces as a silent refusal rather
// than a hard fail, but the URL is NOT handed to the OS.
//
//nolint:gosec,noctx // best-effort browser launch — user-supplied URL is the whole point of the command (after the scheme gate above), and there's no caller-supplied context to thread (failures are silently ignored upstream).
func (o *OSOpener) Open(rawURL string) error {
	name, args := o.CommandFor(rawURL)
	if name == "" {
		return ErrUnopenableURL
	}
	return exec.Command(name, args...).Start()
}

// NoopOpener does nothing. Useful as a default in headless environments
// or when an upstream wants to prevent any browser launch entirely.
type NoopOpener struct{}

// Open returns nil without doing anything.
func (NoopOpener) Open(_ string) error { return nil }
