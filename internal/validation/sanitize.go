// Package validation provides input hardening, fuzzy option correction, and
// JSON Schema validation for Urlbox API payloads.
package validation

import (
	"github.com/urlbox/urlbox-cli/internal/output"
)

// MaxPayloadBytes is the upper bound for raw --json payloads (1 MiB).
const MaxPayloadBytes = 1 << 20

// SanitizeRaw enforces payload-level limits before JSON parsing.
// Returns nil on pass, *output.CLIError with code "validation" on failure.
func SanitizeRaw(b []byte) *output.CLIError {
	if len(b) > MaxPayloadBytes {
		return output.NewCLIError(
			output.ErrValidation,
			"Payload exceeds maximum size of 1 MiB",
			"Payloads are capped at 1 MiB. Trim unused fields or split into multiple renders.",
		)
	}
	return nil
}

// SanitizeStringField rejects control characters (below 0x20 or equal to 0x7F)
// in user-supplied string fields.
func SanitizeStringField(name, value string) *output.CLIError {
	for _, r := range value {
		if r < 0x20 || r == 0x7F {
			return output.NewCLIError(
				output.ErrValidation,
				`Field "`+name+`" contains a control character`,
				"Strip control characters (newline, tab, escape, DEL) before sending.",
			)
		}
	}
	return nil
}
