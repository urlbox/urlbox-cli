// internal/config/apihost_validate.go — Round 8 Class B (GG): single
// gate for api_host values. Before this commit, every write/read site
// accepted any string verbatim, including:
//
//   - javascript:alert(1), file:///etc/passwd, ftp://evil (hostile
//     schemes — wouldn't actually exploit since the CLI never loads
//     these URLs in a browser, but they signal "you're being phished
//     by a config edit" and should be rejected)
//   - https://user:pass@evil.com (embedded credentials — leaks via
//     Authorization headers in some clients, also a classic phishing
//     redirect pattern)
//   - "https://api.urlbox.com\r\nX-Evil: 1" (CRLF — header-injection
//     primer once the host is concatenated into a request URL)
//   - "" (empty after trim — silent fallback to default; legitimate
//     "unset" is handled by NOT calling config set api_host, not by
//     setting it to empty)
//
// The rule is intentionally tight because api_host is rarely set in
// practice (production users don't override it), so a strict allowlist
// has near-zero false-positive cost.
package config

import (
	"net/url"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// ValidateAPIHost returns the cleaned value or a CLIError. Rules:
//
//   - Must trim to non-empty.
//   - Must parse as a URL (net/url.Parse).
//   - Scheme must be http or https. Other schemes are rejected.
//   - No userinfo (no embedded credentials).
//   - Host (after url.Parse normalises) must be non-empty.
//   - No control characters anywhere in the raw value (rejects CRLF
//     before url.Parse silently strips them).
//   - No fragment, no query (this is a base URL, not a request URL).
//
// Returns the cleaned value (trimmed, fragment/query trailing slash
// preserved as the user supplied them).
func ValidateAPIHost(raw string) (string, *output.CLIError) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", output.NewCLIError(
			output.ErrUsage,
			"api_host cannot be empty",
			"Either set a non-empty https URL (e.g. https://api.urlbox.com) or omit api_host entirely so the CLI uses the default. To unset a per-profile api_host, recreate the profile without --api-host.",
		)
	}
	// Trim first (paste safety), THEN reject control chars — the actual
	// header-injection risk is interior CRLF, not a trailing newline
	// from a copy-paste.
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7F {
			return "", output.NewCLIError(
				output.ErrUsage,
				"api_host contains a control character",
				"api_host must be a plain http(s) URL — embedded newlines / tabs / null bytes are usually paste corruption or a header-injection attempt. Re-enter the value.",
			)
		}
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", output.NewCLIError(
			output.ErrUsage,
			"api_host is not a valid URL: "+err.Error(),
			"api_host must look like https://api.urlbox.com — scheme, then host, optional port.",
		)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", output.NewCLIError(
			output.ErrUsage,
			`api_host scheme must be http or https, got "`+u.Scheme+`"`,
			"The Urlbox API only speaks HTTP(S). Schemes like javascript:, file://, ftp:// are rejected as either paste corruption or a phishing attempt.",
		)
	}
	if u.User != nil {
		return "", output.NewCLIError(
			output.ErrUsage,
			"api_host must not include userinfo (credentials in the URL)",
			"Pass credentials via --api-key/--api-secret, URLBOX_API_SECRET, or a config profile — never embed them in api_host. https://user:pass@host leaks via logs and Referer headers.",
		)
	}
	if u.Host == "" {
		return "", output.NewCLIError(
			output.ErrUsage,
			"api_host has no host component",
			"api_host must include a hostname (e.g. https://api.urlbox.com), not just a scheme.",
		)
	}
	if u.Fragment != "" || u.RawQuery != "" {
		return "", output.NewCLIError(
			output.ErrUsage,
			"api_host must not contain a query string or fragment",
			"api_host is a base URL, not a request URL. Strip ?... and #... before saving.",
		)
	}
	return trimmed, nil
}
