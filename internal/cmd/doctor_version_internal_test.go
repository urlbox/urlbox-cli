package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func withRelease(t *testing.T, current, latest string, err error) {
	t.Helper()
	SetUpdateCheckForTest(current, func(context.Context) (string, error) { return latest, err })
	t.Cleanup(ResetUpdateCheckForTest)
}

func TestCheckVersion_NewerReleaseWarns(t *testing.T) {
	withRelease(t, "1.2.0", "1.3.0", nil)
	c := checkVersion(context.Background())
	if c.Status != "warn" || !strings.Contains(c.Message, "1.3.0 available") || !strings.Contains(c.Hint, "urlbox upgrade") {
		t.Errorf("got %+v", c)
	}
}

func TestCheckVersion_CurrentIsOK(t *testing.T) {
	withRelease(t, "1.3.0", "1.3.0", nil)
	c := checkVersion(context.Background())
	if c.Status != "ok" || c.Message != "1.3.0 (latest)" {
		t.Errorf("got %+v", c)
	}
}

// A failed lookup must not turn a healthy install into a warning — doctor
// is used as a CI health gate.
func TestCheckVersion_LookupFailsStaysOK(t *testing.T) {
	withRelease(t, "1.2.0", "", errors.New("offline"))
	c := checkVersion(context.Background())
	if c.Status != "ok" || !strings.Contains(c.Message, "couldn't check for updates") {
		t.Errorf("got %+v", c)
	}
}

func TestCheckVersion_DevBuildSkipsLookup(t *testing.T) {
	called := false
	SetUpdateCheckForTest("dev", func(context.Context) (string, error) { called = true; return "1.3.0", nil })
	t.Cleanup(ResetUpdateCheckForTest)
	c := checkVersion(context.Background())
	if called || c.Status != "ok" || c.Message != "dev" {
		t.Errorf("called=%v check=%+v", called, c)
	}
}
