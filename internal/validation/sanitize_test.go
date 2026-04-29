// internal/validation/sanitize_test.go
package validation_test

import (
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/validation"
)

func TestSanitizeRaw_AcceptsSmallPayload(t *testing.T) {
	if err := validation.SanitizeRaw([]byte(`{"url":"https://example.com"}`)); err != nil {
		t.Fatalf("expected nil, got: %+v", err)
	}
}

func TestSanitizeRaw_AcceptsExactlyMaxBytes(t *testing.T) {
	// Build a payload that's exactly MaxPayloadBytes long.
	body := `{"url":"` + strings.Repeat("a", validation.MaxPayloadBytes-len(`{"url":""}`)) + `"}`
	if len(body) != validation.MaxPayloadBytes {
		t.Fatalf("test setup wrong: payload is %d bytes, expected %d", len(body), validation.MaxPayloadBytes)
	}
	if err := validation.SanitizeRaw([]byte(body)); err != nil {
		t.Fatalf("expected nil at exactly MaxPayloadBytes, got: %+v", err)
	}
}

func TestSanitizeRaw_RejectsOverMaxBytes(t *testing.T) {
	body := make([]byte, validation.MaxPayloadBytes+1)
	for i := range body {
		body[i] = 'a'
	}
	err := validation.SanitizeRaw(body)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Code != output.ErrValidation {
		t.Errorf("code=%v, want %v", err.Code, output.ErrValidation)
	}
	if err.Message != "Payload exceeds maximum size of 1 MiB" {
		t.Errorf("message=%q", err.Message)
	}
	if !strings.Contains(err.Hint, "1 MiB") {
		t.Errorf("hint should mention 1 MiB; got: %s", err.Hint)
	}
}

func TestSanitizeStringField_RejectsControlChars(t *testing.T) {
	cases := map[string]string{
		"newline":     "https://example.com\nfoo",
		"tab":         "https://example.com\tfoo",
		"null byte":   "https://example.com\x00",
		"escape":      "https://example.com\x1b[0m",
		"bell (0x07)": "https://example.com\x07",
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			err := validation.SanitizeStringField("url", value)
			if err == nil {
				t.Fatalf("expected error for %q", name)
			}
			if err.Code != output.ErrValidation {
				t.Errorf("code=%v, want %v", err.Code, output.ErrValidation)
			}
			if err.Message != `Field "url" contains a control character` {
				t.Errorf("message=%q", err.Message)
			}
			if !strings.Contains(err.Hint, "control characters") {
				t.Errorf("hint should mention control characters; got: %s", err.Hint)
			}
		})
	}
}

func TestSanitizeStringField_AllowsPrintableASCIIAndUnicode(t *testing.T) {
	cases := []string{
		"https://example.com/path?q=1",
		"https://例子.test/路径", // Unicode
		"hello world",        // space (0x20) is allowed
		"with-tilde~chars",
	}
	for _, value := range cases {
		t.Run(value, func(t *testing.T) {
			if err := validation.SanitizeStringField("url", value); err != nil {
				t.Errorf("unexpected error: %+v", err)
			}
		})
	}
}

func TestSanitizeStringField_AllowsDel0x7F(t *testing.T) {
	// 0x7F (DEL) is a control char per ASCII. We reject it too — agents shouldn't
	// be sending DEL in URLs.
	err := validation.SanitizeStringField("url", "https://example.com\x7f")
	if err == nil {
		t.Fatal("expected DEL (0x7F) to be rejected")
	}
}
