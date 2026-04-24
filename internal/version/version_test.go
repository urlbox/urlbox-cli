package version_test

import (
	"testing"

	"github.com/urlbox/urlbox-cli/internal/version"
)

func TestVersionDefaults(t *testing.T) {
	// These defaults are used when building without ldflags (dev builds).
	// Production builds override them via -X ldflags.
	if version.Version != "dev" {
		t.Errorf("expected default Version to be 'dev', got %q", version.Version)
	}
	if version.Commit != "none" {
		t.Errorf("expected default Commit to be 'none', got %q", version.Commit)
	}
	if version.Date != "unknown" {
		t.Errorf("expected default Date to be 'unknown', got %q", version.Date)
	}
}
