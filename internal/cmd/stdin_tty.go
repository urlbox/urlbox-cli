package cmd

import (
	"io"
	"os"

	"golang.org/x/term"
)

var stdinTTYOverride *bool

// SetStdinTTYForTest forces stdin TTY detection for tests.
func SetStdinTTYForTest(v bool) { stdinTTYOverride = &v }

// ResetStdinTTYForTest clears the stdin override.
func ResetStdinTTYForTest() { stdinTTYOverride = nil }

func isStdinTTY(r io.Reader) bool {
	if stdinTTYOverride != nil {
		return *stdinTTYOverride
	}
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(int(f.Fd())) //nolint:gosec // file descriptors fit in int on every platform Go supports
	}
	return false
}
