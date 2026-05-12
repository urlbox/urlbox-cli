// internal/config/secret_validate.go — Round 8 FF: moved from
// internal/cmd/secret_validate.go so config.Resolve can call it on the
// URLBOX_API_SECRET env path. The cmd-side wrapper (still named
// validateSecretValue in package cmd) is now a one-line forward to
// config.ValidateSecretValue.
//
// History:
//   - Round 6 X: introduced as the single gate for direct-input secret
//     paths (auth flag, stdin, file, config set api_secret, profile create).
//   - Round 7 DD: extended to reject Unicode Cf "Format" chars
//     (invisible: zero-width, BOM, bidi).
//   - Round 8 FF: extended to reject invalid UTF-8 and Mn "Mark,
//     Nonspacing" (combining marks, variation selectors) plus moved
//     here so the URLBOX_API_SECRET env path also enforces the rule.
package config

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// ValidateSecretValue is the single gate every secret-writing/-reading
// path goes through. See the file-level comment for history.
//
// Rule:
//
//   - Bytes must be valid UTF-8. Lone surrogates, overlong encodings,
//     bare 0xFE/0xFF, truncated multi-byte sequences are rejected.
//   - Surrounding whitespace is trimmed (paste safety). TrimSpace covers
//     Unicode space categories (Zs/Zl/Zp).
//   - After trim, the value must be non-empty.
//   - The trimmed value must not contain ASCII control characters
//     (0x00-0x1F), Cf "Format" chars (zero-width, BOM, bidi marks),
//     or Mn "Mark, Nonspacing" chars (combining marks, variation
//     selectors). These are invisible by definition — no legitimate
//     API secret contains them.
//
// Returns the cleaned value on success; *output.CLIError (ErrUsage) on
// rejection. The error hint names the specific violation.
func ValidateSecretValue(raw string) (string, *output.CLIError) {
	if !utf8.ValidString(raw) {
		return "", output.NewCLIError(
			output.ErrUsage,
			"API secret contains invalid UTF-8 bytes",
			"Real Urlbox secrets are ASCII alphanumeric — invalid byte sequences (lone surrogates, overlong encodings, raw 0xFF) usually mean a paste artefact or piping from a tool that didn't quote properly. Re-copy from the dashboard.",
		)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", output.NewCLIError(
			output.ErrUsage,
			"API secret cannot be empty or whitespace-only",
			"Provide a non-empty value. The secret is on your project dashboard at https://urlbox.com/dashboard/projects.",
		)
	}
	for _, r := range trimmed {
		if r < 0x20 {
			return "", output.NewCLIError(
				output.ErrUsage,
				"API secret contains a control character",
				"Real Urlbox secrets are alphanumeric tokens — a control character here usually means a paste artefact (embedded newline / tab / null byte). Re-copy from the dashboard.",
			)
		}
		if unicode.In(r, unicode.Cf) {
			return "", output.NewCLIError(
				output.ErrUsage,
				"API secret contains an invisible Unicode character",
				"Real Urlbox secrets are ASCII alphanumeric — a zero-width space, joiner, BOM, or bidi mark here usually means a paste artefact (often from copying out of a styled document or web page). Re-copy from the dashboard.",
			)
		}
		if unicode.In(r, unicode.Mn) {
			return "", output.NewCLIError(
				output.ErrUsage,
				"API secret contains a combining mark or variation selector",
				"Real Urlbox secrets are ASCII alphanumeric — a combining mark (e.g. \"à\" composed as \"a\" + U+0300) or variation selector is invisible in the terminal but changes the secret bytes. This usually comes from pasting through macOS smart-diacritic processing or a styled web page. Re-copy from the dashboard.",
			)
		}
	}
	return trimmed, nil
}
