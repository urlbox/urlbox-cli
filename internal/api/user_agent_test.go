package api_test

import (
	"regexp"
	"runtime"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func TestUserAgent_FormatPinned(t *testing.T) {
	got := api.BuildUserAgent("1.2.3")
	want := "urlbox-cli/1.2.3 (" + runtime.GOOS + "/" + runtime.GOARCH + ") go/" + runtime.Version()
	if got != want {
		t.Errorf("UA=%q\nwant %q", got, want)
	}
}

func TestUserAgent_EmptyVersion_FallsBackToZeroDev(t *testing.T) {
	got := api.BuildUserAgent("")
	re := regexp.MustCompile(`^urlbox-cli/0\.0\.0-dev \(\S+/\S+\) go/go\S+$`)
	if !re.MatchString(got) {
		t.Errorf("UA=%q does not match %s", got, re)
	}
}
