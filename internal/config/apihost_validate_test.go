package config_test

import (
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
)

// TestValidateAPIHost_RejectsHostileSchemes pins Round 8 Class B (GG):
// the adversarial repro accepted javascript:, file://, ftp://, and
// embedded-credential URLs verbatim. Now rejected as ErrUsage.
func TestValidateAPIHost_RejectsHostileSchemes(t *testing.T) {
	cases := []string{
		"javascript:alert(1)",
		"file:///etc/passwd",
		"ftp://evil.com",
		"data:text/html,<script>",
		"chrome://settings",
		"about:blank",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := config.ValidateAPIHost(c)
			if err == nil {
				t.Fatalf("ValidateAPIHost(%q) should error", c)
			}
			if err.Code != "usage" {
				t.Errorf("code=%q, want usage", err.Code)
			}
		})
	}
}

func TestValidateAPIHost_RejectsEmbeddedCredentials(t *testing.T) {
	cases := []string{
		"https://user:pass@evil.com",
		"https://user@evil.com",
		"http://u:p@host.example",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := config.ValidateAPIHost(c)
			if err == nil {
				t.Fatalf("ValidateAPIHost(%q) should error — has userinfo", c)
			}
		})
	}
}

func TestValidateAPIHost_RejectsControlChars(t *testing.T) {
	cases := []string{
		"https://api.urlbox.com\r\nX-Evil: 1",
		"https://api.urlbox.com\nfoo",
		"https://api\turlbox.com",
		"https://api.urlbox.com\x00",
		"https://api.urlbox.com\x07",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := config.ValidateAPIHost(c)
			if err == nil {
				t.Fatalf("ValidateAPIHost(%q) should error — control char", c)
			}
		})
	}
}

func TestValidateAPIHost_RejectsEmptyAndMalformed(t *testing.T) {
	cases := []struct {
		name, in string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no scheme", "api.urlbox.com"},
		{"scheme only", "https://"},
		{"trailing query", "https://api.urlbox.com?foo=bar"},
		{"trailing fragment", "https://api.urlbox.com#anchor"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := config.ValidateAPIHost(c.in)
			if err == nil {
				t.Fatalf("ValidateAPIHost(%q) should error", c.in)
			}
		})
	}
}

func TestValidateAPIHost_AcceptsValid(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"production https", "https://api.urlbox.com", "https://api.urlbox.com"},
		{"local http", "http://localhost:3000", "http://localhost:3000"},
		{"https with port", "https://api.example.com:8443", "https://api.example.com:8443"},
		{"trailing slash preserved", "https://api.urlbox.com/", "https://api.urlbox.com/"},
		{"path segment preserved", "https://api.urlbox.com/v1", "https://api.urlbox.com/v1"},
		{"leading whitespace trimmed", "  https://api.urlbox.com", "https://api.urlbox.com"},
		{"trailing newline trimmed", "https://api.urlbox.com\n", "https://api.urlbox.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := config.ValidateAPIHost(c.in)
			if err != nil {
				t.Fatalf("ValidateAPIHost(%q) errored: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// ─── Class 1.2 (v1.0.4) — http:// only for loopback ─────────────────
//
// Invariant: plain http:// is rejected unless the host is loopback
// (127.0.0.1, ::1, localhost). Closes a downgrade path where a
// careless URLBOX_API_HOST or a hostile overlay turned the Authorization
// header cleartext. Loopback remains permitted so httptest-based
// integration tests (which always use 127.0.0.1) keep working without
// requiring TLS termination.

func TestValidateAPIHost_RejectsPlainHTTPRemoteHost(t *testing.T) {
	cases := []string{
		"http://attacker.example",
		"http://api.urlbox.com", // even our own host: HTTPS only on the wire
		"http://10.0.0.1",
		"http://192.168.1.1",
		"http://example.com:8080",
		"http://8.8.8.8",
		"http://my-dev-box.local",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := config.ValidateAPIHost(c)
			if err == nil {
				t.Fatalf("ValidateAPIHost(%q) should error — plain http to non-loopback", c)
			}
			if err.Code != "usage" {
				t.Errorf("code=%q, want usage", err.Code)
			}
		})
	}
}

func TestValidateAPIHost_AllowsHTTPLoopback(t *testing.T) {
	cases := []string{
		"http://127.0.0.1",
		"http://127.0.0.1:8080",
		"http://127.0.0.1:65535",
		"http://[::1]",
		"http://[::1]:9000",
		"http://localhost",
		"http://localhost:3000",
		"http://LocalHost", // case-insensitive
		"http://LOCALHOST:8000",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, err := config.ValidateAPIHost(c)
			if err != nil {
				t.Errorf("ValidateAPIHost(%q) should pass — loopback dev host; got %v", c, err)
			}
		})
	}
}
