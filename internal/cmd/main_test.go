package cmd

import (
	"os"
	"testing"
	"time"
)

// TestMain disables the session-client retry backoff for the whole package.
//
// The session retry tests assert how many attempts were made, never how long
// they took, so the real 1s/2s/4s budget bought nothing but wall-clock: the
// package went from ~6s to ~40s. Only session clients are affected — the
// render and status retry paths keep their production sleep.
func TestMain(m *testing.M) {
	sessionRetrySleep = func(time.Duration) {}
	os.Exit(m.Run())
}
