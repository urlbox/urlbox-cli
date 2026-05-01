// internal/validation/validate_test.go
package validation_test

import (
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/validation"
)

func TestValidatePayload_AcceptsValidPayload(t *testing.T) {
	out, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","format":"png","width":1920}`))
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
	_, err := validation.ValidatePayload(body)
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != "Payload exceeds maximum size of 1 MiB" {
		t.Errorf("message=%q", err.Message)
	}
}

func TestValidatePayload_RejectsMalformedJSON(t *testing.T) {
	_, err := validation.ValidatePayload([]byte(`{"url":}`))
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

func TestValidatePayload_SuggestsCorrection_OneUnknown(t *testing.T) {
	_, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","fullPage":true}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != `Unknown option: fullPage` {
		t.Errorf("message=%q", err.Message)
	}
	if err.Hint != `Did you mean "full_page"?` {
		t.Errorf("hint=%q", err.Hint)
	}
}

func TestValidatePayload_SuggestsCorrection_MultipleUnknown(t *testing.T) {
	_, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","fullPage":true,"widht":1920}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != "Unknown options: fullPage, widht" {
		t.Errorf("message=%q", err.Message)
	}
	if err.Hint != `Did you mean: fullPage → "full_page", widht → "width"?` {
		t.Errorf("hint=%q", err.Hint)
	}
}

func TestValidatePayload_UnknownNoNearMatch(t *testing.T) {
	_, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","xyzzy":true}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != "Unknown option: xyzzy" {
		t.Errorf("message=%q", err.Message)
	}
	if err.Hint != "Run `urlbox schema render` to see all valid options." {
		t.Errorf("hint=%q", err.Hint)
	}
}

func TestValidatePayload_RejectsURLControlChars(t *testing.T) {
	_, err := validation.ValidatePayload([]byte(`{"url":"https://example.com\nfoo"}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if err.Message != `Field "url" contains a control character` {
		t.Errorf("message=%q", err.Message)
	}
}

func TestValidatePayload_FailsSchemaTypeMismatch(t *testing.T) {
	// width per the public schema is anyOf [string, integer with int53 bounds].
	// A string value should ACCEPT (the API coerces). Passing an *object* (or
	// boolean) for width fails the schema. Use an object so neither anyOf branch
	// matches.
	_, err := validation.ValidatePayload([]byte(`{"url":"https://example.com","width":{"obj":true}}`))
	if err == nil || err.Code != output.ErrValidation {
		t.Fatalf("expected validation error, got: %+v", err)
	}
	if !strings.HasPrefix(err.Message, "Payload validation failed") {
		t.Errorf("message=%q", err.Message)
	}
	if !strings.Contains(err.Hint, "width") {
		t.Errorf("hint should mention width; got: %s", err.Hint)
	}
}

// Regression guard: the dashboard exposes video_scroll (+ four supporting
// fields). The CLI's schema must accept them so users can drive
// video_scroll renders end-to-end. See urlbox-mono
// packages/types/src/render/render.types.ts:1277-1292 for API definitions.
//
// Background: a one-shot generation from feature/urlbox-cli left these
// fields out of schema/render.json. The auto-PR sync workflow that should
// have caught this never landed on urlbox-mono main (the dashboard was
// refactored and the allowlist source moved). v0.8.1 hand-patches the
// gap; the broader sync-pipeline rebuild is tracked separately.
func TestValidatePayload_AcceptsVideoScrollFields(t *testing.T) {
	payload := []byte(`{
		"url": "https://example.com",
		"format": "mp4",
		"video_scroll": true,
		"video_scroll_back": true,
		"video_scroll_distance": 800,
		"video_scroll_duration": 900,
		"video_scroll_back_duration": 600
	}`)
	if _, err := validation.ValidatePayload(payload); err != nil {
		t.Fatalf("ValidatePayload rejected video_scroll fields: %+v", err)
	}
}
