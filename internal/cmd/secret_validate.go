// internal/cmd/secret_validate.go
package cmd

import (
	"strings"
	"unicode"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// validateSecretValue is the single gate every secret-writing path goes
// through: auth, config set api_secret, config profile create. Round 6
// adversarial found four sibling bypasses of the auth overwrite guard
// because each entry point had its own ad-hoc validation (or none).
// Centralizing the rule prevents that class of siblings. Round 7 added
// invisible-Unicode rejection — strings.TrimSpace already handles Zs/Zl/
// Zp categories (NBSP, ideographic space, line/paragraph separators) but
// Cf "Format" chars (zero-width space, ZWNJ, ZWJ, word joiner, BOM, LRM/
// RLM, bidi embeddings/overrides/isolates, Mongolian vowel separator)
// are invisible in terminals and silently corrupt stored secrets,
// causing mysterious 401s ("but I copied the right secret!").
//
// Rule:
//
//   - Surrounding whitespace is trimmed (paste safety — newlines and
//     stray spaces from copy/paste shouldn't silently end up in the
//     stored secret). TrimSpace covers Unicode space categories.
//   - After trim, the value must be non-empty.
//   - The trimmed value must not contain ASCII control characters
//     (0x00-0x1F) anywhere. Real Urlbox secrets are alphanumeric tokens;
//     control chars here are either bugs (paste corruption) or attacks
//     (CRLF injection, null-byte truncation).
//   - The trimmed value must not contain any Unicode "Format" (Cf)
//     character anywhere. These are invisible by definition — no
//     legitimate API secret contains one.
//
// Returns the cleaned value on success; *output.CLIError (ErrUsage) on
// rejection. The error hint names the specific violation.
func validateSecretValue(raw string) (string, *output.CLIError) {
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
	}
	return trimmed, nil
}
