// internal/cmd/auth_preflight.go — v1.0.4 Class 5.
//
// Client-side pre-flight checks so predictable failures fail fast with
// the CLI's own vocabulary, not the API's. Pre-1.0.4 a missing-secret
// `urlbox render <url>` returned the API's confusing
// "Api Key does not exist" — vocabulary mismatch (the CLI says "API
// secret" everywhere) and a wasted network round-trip.
package cmd

import (
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// requireSecret returns a CLI auth error if no API secret resolved
// through any source (flag, env, repo overlay, profile). Detected
// client-side so users don't burn a network round-trip on the API's
// "Api Key does not exist" message — and the CLI's vocabulary
// ("API secret") stays consistent across error paths.
//
// Returns nil when resolved.APISecret is non-empty (the caller proceeds).
func requireSecret(resolved *config.Resolved) *output.CLIError {
	if resolved != nil && resolved.APISecret != "" {
		return nil
	}
	return output.NewCLIError(
		output.ErrAuth,
		"no API secret configured",
		"Run `urlbox auth --api-secret <secret>` to store one, or set URLBOX_API_SECRET in the environment. Get your secret from https://urlbox.com/dashboard/projects.",
	)
}
