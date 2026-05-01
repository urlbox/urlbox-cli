package cmd

// White-box tests (package cmd, not cmd_test) so we can call unexported buildCurl.

import (
	"strings"
	"testing"
)

func TestBuildCurl_PinnedOutput_AlphaSorted_RedactedSecret(t *testing.T) {
	opts := map[string]any{
		"url":    "https://example.com",
		"format": "png",
		"width":  1920,
	}
	got := buildCurl("https://api.urlbox.com", "/v1/screenshot", opts)
	want := `curl -X POST 'https://api.urlbox.com/v1/screenshot' -H 'Authorization: Bearer $URLBOX_API_SECRET' -H 'Content-Type: application/json' -d '{"format":"png","url":"https://example.com","width":1920}'`
	if got != want {
		t.Errorf("\nGOT:  %s\nWANT: %s", got, want)
	}
}

func TestBuildCurl_NestedObject_Preserved(t *testing.T) {
	opts := map[string]any{
		"url":        "https://example.com",
		"thumbnails": []any{map[string]any{"preset": "sm"}},
	}
	got := buildCurl("https://api.urlbox.com", "/v1/screenshot", opts)
	want := `curl -X POST 'https://api.urlbox.com/v1/screenshot' -H 'Authorization: Bearer $URLBOX_API_SECRET' -H 'Content-Type: application/json' -d '{"thumbnails":[{"preset":"sm"}],"url":"https://example.com"}'`
	if got != want {
		t.Errorf("\nGOT:  %s\nWANT: %s", got, want)
	}
}

func TestBuildCurl_SingleQuoteInValue_Escaped(t *testing.T) {
	opts := map[string]any{"url": "https://example.com/?q='foo'"}
	got := buildCurl("https://api.urlbox.com", "/v1/screenshot", opts)
	// Standard sh-safe escape: replace ' with '\''
	if !strings.Contains(got, `'\''`) {
		t.Errorf("expected '\\''  escape sequence; got %s", got)
	}
}

func TestBuildCurl_AsyncPath(t *testing.T) {
	opts := map[string]any{"url": "https://example.com"}
	got := buildCurl("https://api.urlbox.com", "/v1/screenshot/async", opts)
	if !strings.Contains(got, "/v1/screenshot/async") {
		t.Errorf("async path missing: %s", got)
	}
}
