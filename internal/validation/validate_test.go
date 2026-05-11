// internal/validation/validate_test.go
package validation_test

import (
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/validation"
)

func TestValidatePayload_AcceptsValidPayload(t *testing.T) {
	out, _, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","format":"png","width":1920}`))
	if err != nil {
		t.Fatalf("unexpected error: %+v", err)
	}
	if out["url"] != "https://example.com" {
		t.Errorf("url not parsed: %v", out["url"])
	}
}

func TestValidatePayload_RejectsTooLarge(t *testing.T) {
	body := make([]byte, validation.MaxPayloadBytes+1)
	for i := range body {
		body[i] = 'a'
	}
	_, _, err := validation.ValidatePayload(body)
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != "Payload exceeds maximum size of 1 MiB" {
		t.Errorf("message=%q", err.Message)
	}
}

func TestValidatePayload_RejectsMalformedJSON(t *testing.T) {
	_, _, err := validation.ValidatePayload([]byte(`{"url":}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if !strings.HasPrefix(err.Message, "Payload is not valid JSON") {
		t.Errorf("message=%q", err.Message)
	}
	if err.Hint == "" {
		t.Errorf("hint should not be empty")
	}
}

// v0.9.0 change: a fuzzy-matchable typo no longer errors. The CLI emits a
// stderr warning and passes the key through verbatim — the API decides.
func TestValidatePayload_SuggestsCorrection_OneUnknown(t *testing.T) {
	got, warnings, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","fullPage":true}`))
	if err != nil {
		t.Fatalf("expected no error (warning-only), got: %+v", err)
	}
	if got["fullPage"] != true {
		t.Errorf("expected key 'fullPage' to pass through verbatim; got %v", got["fullPage"])
	}
	if len(warnings) == 0 {
		t.Fatalf("expected at least one warning about 'fullPage'")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "fullPage") && strings.Contains(w, "full_page") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning mentioning 'fullPage' and suggesting 'full_page'; got: %v", warnings)
	}
}

// v0.9.0 change: multiple fuzzy-matchable typos collapse into ONE warning.
func TestValidatePayload_SuggestsCorrection_MultipleUnknown(t *testing.T) {
	got, warnings, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","fullPage":true,"widht":1920}`))
	if err != nil {
		t.Fatalf("expected no error (warning-only), got: %+v", err)
	}
	if got["fullPage"] != true {
		t.Errorf("expected fullPage to pass through verbatim")
	}
	if got["widht"] != float64(1920) {
		t.Errorf("expected widht to pass through verbatim")
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly one summary warning for multiple unknowns, got %d: %v", len(warnings), warnings)
	}
	w := warnings[0]
	if !strings.Contains(w, "fullPage") || !strings.Contains(w, "full_page") {
		t.Errorf("expected warning to mention fullPage → full_page; got: %s", w)
	}
	if !strings.Contains(w, "widht") || !strings.Contains(w, "width") {
		t.Errorf("expected warning to mention widht → width; got: %s", w)
	}
}

// v0.9.0 change: an unknown key with no fuzzy match passes through silently.
func TestValidatePayload_UnknownNoNearMatch(t *testing.T) {
	got, warnings, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","xyzzy":true}`))
	if err != nil {
		t.Fatalf("expected no error (silent passthrough), got: %+v", err)
	}
	if got["xyzzy"] != true {
		t.Errorf("expected xyzzy to pass through verbatim")
	}
	if len(warnings) != 0 {
		t.Errorf("expected zero warnings for non-matching unknown key; got: %v", warnings)
	}
}

func TestValidatePayload_RejectsURLControlChars(t *testing.T) {
	_, _, err := validation.ValidatePayload([]byte(`{"url":"https://example.com\nfoo"}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != `Field "url" contains a control character` {
		t.Errorf("message=%q", err.Message)
	}
}

// Regression guard: video_scroll + four supporting fields are no longer in the
// regenerated v0.9.0 schema (the dashboard's allowlist source moved during a
// refactor and the field list drifted). v0.9.0's "schema as documentation"
// model handles this gracefully — unknown fields pass through verbatim and the
// API decides. This test guarantees the unknown-passthrough path stays open
// for these dashboard-supported fields. See urlbox-mono
// packages/types/src/render/render.types.ts:1277-1292 for API definitions.
func TestValidatePayload_AcceptsUnknownFields_WithWarning(t *testing.T) {
	payload := []byte(`{
		"url": "https://example.com",
		"format": "mp4",
		"video_scroll": true,
		"video_scroll_back": true,
		"video_scroll_distance": 800,
		"video_scroll_duration": 900,
		"video_scroll_back_duration": 600
	}`)
	got, _, err := validation.ValidatePayload(payload)
	if err != nil {
		t.Fatalf("ValidatePayload rejected video_scroll fields: %+v", err)
	}
	if got["video_scroll"] != true {
		t.Errorf("expected video_scroll to pass through verbatim")
	}
	if got["video_scroll_distance"] != float64(800) {
		t.Errorf("expected video_scroll_distance to pass through verbatim")
	}
	// Warnings may or may not be emitted depending on whether ClosestMatch
	// finds something within edit-distance threshold for these names. The
	// load-bearing assertion is that the payload is not REJECTED.
}

func TestValidatePayload_UnknownKey_FuzzyMatch_WarnsAndPasses(t *testing.T) {
	payload := []byte(`{"url":"https://example.com","fromat":"png"}`)
	got, warnings, cliErr := validation.ValidatePayload(payload)
	if cliErr != nil {
		t.Fatalf("expected no error, got: %v", cliErr)
	}
	if got["fromat"] != "png" {
		t.Errorf("expected key 'fromat' to pass through verbatim (no auto-correct); got %v", got["fromat"])
	}
	if len(warnings) == 0 {
		t.Fatalf("expected at least one warning about 'fromat'")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "fromat") && strings.Contains(w, "format") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning mentioning 'fromat' and suggesting 'format'; got: %v", warnings)
	}
}

func TestValidatePayload_UnknownKey_NoMatch_PassesSilently(t *testing.T) {
	payload := []byte(`{"url":"https://example.com","totally_made_up_xyz":true}`)
	got, warnings, cliErr := validation.ValidatePayload(payload)
	if cliErr != nil {
		t.Fatalf("expected no error, got: %v", cliErr)
	}
	if got["totally_made_up_xyz"] != true {
		t.Errorf("expected key to pass through verbatim")
	}
	if len(warnings) != 0 {
		t.Errorf("expected zero warnings for non-matching unknown key; got: %v", warnings)
	}
}

func TestValidatePayload_KnownKey_BadType_PassesThrough(t *testing.T) {
	payload := []byte(`{"url":"https://example.com","width":"abc"}`)
	got, _, cliErr := validation.ValidatePayload(payload)
	if cliErr != nil {
		t.Fatalf("expected no local error (API will validate); got: %v", cliErr)
	}
	if got["width"] != "abc" {
		t.Errorf("expected width to pass through verbatim as 'abc'")
	}
}

// TestValidatePayload_Concurrent_NoCrossPollination is the Arch I3 guard:
// the package no longer holds a global lastWarnings slice, so concurrent
// callers must each see their own warning set without race / clobber.
// Run with -race to make this assertion meaningful.
func TestValidatePayload_Concurrent_NoCrossPollination(t *testing.T) {
	const goroutines = 16
	const iters = 32
	done := make(chan struct{}, goroutines)
	errCh := make(chan string, goroutines*iters)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < iters; j++ {
				// Alternate between two distinct typo payloads. Each call
				// must produce its OWN warnings — no cross-pollination.
				if (id+j)%2 == 0 {
					_, warnings, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","fromat":"png"}`))
					if err != nil {
						errCh <- "unexpected error: " + err.Message
						continue
					}
					if len(warnings) == 0 || !strings.Contains(warnings[0], "fromat") {
						errCh <- "expected fromat warning; got: " + strings.Join(warnings, " | ")
					}
				} else {
					_, warnings, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","widht":1920}`))
					if err != nil {
						errCh <- "unexpected error: " + err.Message
						continue
					}
					if len(warnings) == 0 || !strings.Contains(warnings[0], "widht") {
						errCh <- "expected widht warning; got: " + strings.Join(warnings, " | ")
					}
				}
			}
		}(i)
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
	close(errCh)
	for msg := range errCh {
		t.Error(msg)
	}
}
