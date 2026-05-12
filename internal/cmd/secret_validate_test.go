// internal/cmd/secret_validate_test.go — class-fix tests for secret-value
// validation. Round 6 surfaced four sibling bypasses of the secret guard:
// whitespace-only --api-secret accepted (auth + config set); control
// chars accepted; config set api_secret ” silently cleared the secret
// (bypassing the auth overwrite guard); leading/trailing whitespace
// silently saved with the padding.
//
// The fix lives in one validateSecretValue helper that every secret-
// writing path now routes through. These tests probe that helper
// directly. Integration tests in auth_test.go and config_test.go then
// confirm each entry point (auth --api-secret, --api-secret-stdin,
// --api-secret-file, config set api_secret, profile create --api-secret)
// gives identical behavior.
package cmd

import (
	"strings"
	"testing"
)

func TestValidateSecretValue_RejectsEmptyAndWhitespace(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"empty string", ""},
		{"spaces only", "   "},
		{"tabs only", "\t\t\t"},
		{"newlines only", "\n\n"},
		{"crlf only", "\r\n\r\n"},
		{"mixed whitespace", " \t \n \r"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateSecretValue(c.in)
			if err == nil {
				t.Fatalf("validateSecretValue(%q) should error", c.in)
			}
			if string(err.Code) != "usage" {
				t.Errorf("code=%q, want usage", err.Code)
			}
		})
	}
}

func TestValidateSecretValue_RejectsControlChars(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"internal newline", "abc\ndef"},
		{"internal carriage return", "abc\rdef"},
		{"internal tab", "abc\tdef"},
		{"null byte", "abc\x00def"},
		{"bell", "abc\x07def"},
		{"escape", "abc\x1bdef"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateSecretValue(c.in)
			if err == nil {
				t.Fatalf("validateSecretValue(%q) should error", c.in)
			}
			if string(err.Code) != "usage" {
				t.Errorf("code=%q, want usage", err.Code)
			}
		})
	}
}

func TestValidateSecretValue_TrimsSurroundingWhitespace(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"leading spaces", "  sec_abc123", "sec_abc123"},
		{"trailing spaces", "sec_abc123  ", "sec_abc123"},
		{"both", "  sec_abc123  ", "sec_abc123"},
		{"trailing newline", "sec_abc123\n", "sec_abc123"},
		{"trailing crlf", "sec_abc123\r\n", "sec_abc123"},
		{"trailing tab", "sec_abc123\t", "sec_abc123"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := validateSecretValue(c.in)
			if err != nil {
				t.Fatalf("validateSecretValue(%q) errored: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestValidateSecretValue_AcceptsNormalSecrets(t *testing.T) {
	cases := []string{
		"sec_abc123",
		"ubx_sk_validlooking",
		"a", // 1 char — degenerate but technically valid
		strings.Repeat("a", 64),
		"sec-with-dashes",
		"sec_with_underscores",
		"sec.with.dots",
		"SEC_UPPERCASE_123",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got, err := validateSecretValue(c)
			if err != nil {
				t.Fatalf("validateSecretValue(%q) errored: %v", c, err)
			}
			if got != c {
				t.Errorf("got %q, want %q (no mutation expected)", got, c)
			}
		})
	}
}
