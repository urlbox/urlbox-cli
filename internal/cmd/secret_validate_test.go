// internal/cmd/secret_validate_test.go — class-fix tests for secret-value
// validation. Round 6 surfaced four sibling bypasses of the secret guard:
// whitespace-only --api-secret accepted (auth + config set); control
// chars accepted; config set api_secret "" silently cleared the secret
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

// TestValidateSecretValue_RejectsInvisibleUnicode pins Round 7 (Med):
// strings.TrimSpace handles Zs/Zl/Zp (NBSP, line/paragraph separators) but
// NOT Cf "Format" characters — zero-width spaces, joiners, BOM, bidi
// controls. These are invisible in terminals but persist verbatim in the
// stored secret, causing auth to fail with mysterious 401s ("but I copied
// the right secret!"). The class-fix rejects every Cf rune anywhere in
// the value.
//
// All invisible chars are spelled with \u escapes so the source file stays
// pure ASCII — embedding raw bytes makes the file unreadable and Go
// rejects a literal U+FEFF outside the first byte of source.
func TestValidateSecretValue_RejectsInvisibleUnicode(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"leading ZWSP (U+200B)", "\u200bsec_abc"},
		{"trailing ZWSP (U+200B)", "sec_abc\u200b"},
		{"internal ZWSP (U+200B)", "sec\u200babc"},
		{"leading BOM (U+FEFF)", "\ufeffsec_abc"},
		{"trailing BOM (U+FEFF)", "sec_abc\ufeff"},
		{"zero-width non-joiner (U+200C)", "sec\u200cabc"},
		{"zero-width joiner (U+200D)", "sec\u200dabc"},
		{"word joiner (U+2060)", "sec\u2060abc"},
		{"left-to-right mark (U+200E)", "sec\u200eabc"},
		{"right-to-left mark (U+200F)", "sec\u200fabc"},
		{"left-to-right embedding (U+202A)", "sec\u202aabc"},
		{"right-to-left override (U+202E)", "sec\u202eabc"},
		{"left-to-right isolate (U+2066)", "sec\u2066abc"},
		{"pop directional isolate (U+2069)", "sec\u2069abc"},
		{"mongolian vowel separator (U+180E)", "sec\u180eabc"},
		{"pure ZWSP only", "\u200b\u200b\u200b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := validateSecretValue(c.in)
			if err == nil {
				t.Fatalf("validateSecretValue(%q) should error — contains invisible Unicode", c.in)
			}
			if string(err.Code) != "usage" {
				t.Errorf("code=%q, want usage", err.Code)
			}
		})
	}
}

// TestValidateSecretValue_TrimsUnicodeWhitespace pins that Unicode space-
// category chars at the edges (NBSP, line sep, paragraph sep, ideographic
// space, en quad) are stripped — same paste-safety contract as ASCII
// spaces. unicode.IsSpace covers Zs/Zl/Zp, which TrimSpace uses, so this
// behavior is already correct — these tests pin it so future "tighten the
// trim" changes can't quietly regress paste-safety.
func TestValidateSecretValue_TrimsUnicodeWhitespace(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"leading NBSP (U+00A0)", "\u00a0sec_abc", "sec_abc"},
		{"trailing NBSP (U+00A0)", "sec_abc\u00a0", "sec_abc"},
		{"leading ideographic space (U+3000)", "\u3000sec_abc", "sec_abc"},
		{"leading en quad (U+2000)", "\u2000sec_abc", "sec_abc"},
		{"trailing line separator (U+2028)", "sec_abc\u2028", "sec_abc"},
		{"trailing paragraph separator (U+2029)", "sec_abc\u2029", "sec_abc"},
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
