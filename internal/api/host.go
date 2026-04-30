package api

import "os"

// DefaultAPIHost is the production Urlbox API base URL.
const DefaultAPIHost = "https://api.urlbox.com"

// EnvAPIHost is the env var name for overriding the API host (used by
// internal testing and by users who run a custom API endpoint).
const EnvAPIHost = "URLBOX_API_HOST"

// ResolveAPIHost returns the env override if set, else DefaultAPIHost.
func ResolveAPIHost() string {
	if h := os.Getenv(EnvAPIHost); h != "" {
		return h
	}
	return DefaultAPIHost
}
