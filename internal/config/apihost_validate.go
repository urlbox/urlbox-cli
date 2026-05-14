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
	"net"
	"net/url"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// ValidateAPIHost returns the cleaned value or a CLIError. Rules:
//
//   - Must trim to non-empty.
//   - Must parse as a URL (net/url.Parse).
//   - Scheme must be https. Plain http:// is permitted ONLY for loopback
//     hosts (127.0.0.1, ::1, localhost) so httptest-based integration
//     tests and local dev servers work without TLS termination. v1.0.4
//     tightened this — pre-1.0.4 any http:// was accepted, making a
//     careless URLBOX_API_HOST or hostile overlay a cleartext-downgrade
//     primitive on the Authorization header.
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
	// v1.0.4 Class 1.2: plain http:// is only allowed for loopback hosts.
	// The Urlbox API endpoint is HTTPS; permitting http:// for arbitrary
	// hosts turned a careless URLBOX_API_HOST or a hostile overlay into a
	// cleartext-downgrade primitive on the Authorization header. Loopback
	// stays permitted because httptest-based integration tests bind to
	// 127.0.0.1 and dev servers commonly run without TLS.
	if u.Scheme == "http" && !isLoopbackHost(u.Hostname()) {
		return "", output.NewCLIError(
			output.ErrUsage,
			"api_host scheme must be https (plain http is only allowed for loopback dev hosts)",
			"The Urlbox API endpoint is HTTPS-only. http:// is permitted for 127.0.0.1, ::1, or localhost so local development and httptest-based integration tests work; for any remote host use https://.",
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

// isLoopbackHost returns true if host is 127.0.0.1, ::1, or localhost
// (case-insensitive). Used by ValidateAPIHost to permit plain http://
// for local development without permitting it for any remote host.
//
// Falls through to net.ParseIP for completeness — a future "10.0.0.1"
// rule could go here, but today the policy is exact-match loopback only
// (matches what httptest.NewServer binds to and what dev servers
// document).
func isLoopbackHost(host string) bool {
	h := strings.ToLower(host)
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
		return true
	}
	return false
}
