package cmd

import (
	"net/url"
	"strings"
)

// maskSecret returns a redacted form of the API secret for safe display.
func maskSecret(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-2:]
}

func maskProxyURL(raw string, reveal bool) string {
	if reveal {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return maskSecret(raw)
	}
	if parsed.User == nil {
		if strings.Contains(raw, "@") {
			return maskSecret(raw)
		}
		return raw
	}
	if _, hasPassword := parsed.User.Password(); hasPassword {
		parsed.User = url.UserPassword(parsed.User.Username(), "****")
	}
	return strings.Replace(parsed.String(), "%2A%2A%2A%2A", "****", 1)
}
