package config

import "github.com/urlbox/urlbox-cli/internal/output"

// Env var names read by the CLI. Constants live here (not in config.go) because
// callers typically grab them when assembling ResolveOptions.
const (
	EnvAPISecret = "URLBOX_API_SECRET" //nolint:gosec // env var name, not a credential
	EnvProfile   = "URLBOX_PROFILE"
	EnvAPIHost   = "URLBOX_API_HOST"

	// DefaultAPIHost is the production endpoint. Phase 3 ships production-host
	// only — users who need a custom host set api_host on a profile.
	DefaultAPIHost = "https://api.urlbox.com"
)

// ResolveOptions is the set of inputs to the priority chain.
//
// Callers populate flag fields from CLI flags, env fields from os.Getenv, and
// pass already-loaded RepoOverlay and Config. Resolve does no I/O of its own.
type ResolveOptions struct {
	FlagAPIKey, FlagAPISecret, FlagAPIHost, FlagProfile string
	EnvAPISecret, EnvProfile, EnvAPIHost                string
	RepoOverlay                                         *RepoOverlay
	Config                                              *Config
}

// Resolved is the flat result of priority resolution. APIHost always has a
// value (DefaultAPIHost when nothing else applies); APIKey/APISecret may be
// empty if no source provided one.
type Resolved struct {
	APIKey, APISecret, APIHost, Profile string
	Source                              Source
}

// Source records the provenance of each field on Resolved. Values are one of:
// "flag" | "env" | "repo" | "profile" | "default_profile" | "default" |
// (empty when the field was not set).
type Source struct {
	APIKey, APISecret, APIHost, Profile string
}

// Resolve flattens opts into a single Resolved.
//
// Errors:
//   - FlagProfile or EnvProfile names a profile that doesn't exist in
//     opts.Config (Round 5 Adv-2 + Round 7 EE).
//   - EnvAPISecret fails ValidateSecretValue (Round 8 FF — env path used
//     to bypass the rule the flag/stdin/file paths enforce, letting
//     malformed bytes flow through to HMAC signing).
//
//nolint:gocritic // ResolveOptions is intentionally passed by value: this is a pure resolver and the input is immutable. A pointer would invite callers to mutate.
func Resolve(opts ResolveOptions) (*Resolved, error) {
	if opts.EnvAPISecret != "" {
		cleaned, vErr := ValidateSecretValue(opts.EnvAPISecret)
		if vErr != nil {
			// Re-frame the error so the user knows the bad value came from
			// the env var, not a --api-secret flag.
			vErr.Message = "URLBOX_API_SECRET: " + vErr.Message
			return nil, vErr
		}
		opts.EnvAPISecret = cleaned
	}
	// Round 8 GG: api_host got no validation at all before this — accepted
	// javascript:, file://, embedded credentials, CRLF. Validate every
	// non-empty external source (env + flag) so the rule is enforced
	// regardless of who set the value.
	if opts.EnvAPIHost != "" {
		cleaned, vErr := ValidateAPIHost(opts.EnvAPIHost)
		if vErr != nil {
			vErr.Message = "URLBOX_API_HOST: " + vErr.Message
			return nil, vErr
		}
		opts.EnvAPIHost = cleaned
	}
	if opts.FlagAPIHost != "" {
		cleaned, vErr := ValidateAPIHost(opts.FlagAPIHost)
		if vErr != nil {
			vErr.Message = "--api-host: " + vErr.Message
			return nil, vErr
		}
		opts.FlagAPIHost = cleaned
	}
	r := &Resolved{}

	switch {
	case opts.FlagProfile != "":
		r.Profile, r.Source.Profile = opts.FlagProfile, "flag"
	case opts.RepoOverlay != nil && opts.RepoOverlay.Profile != "":
		r.Profile, r.Source.Profile = opts.RepoOverlay.Profile, "repo"
	case opts.EnvProfile != "":
		r.Profile, r.Source.Profile = opts.EnvProfile, "env"
	case opts.Config != nil && opts.Config.DefaultProfile != "":
		r.Profile, r.Source.Profile = opts.Config.DefaultProfile, "default_profile"
	default:
		r.Profile, r.Source.Profile = "default", "default"
	}

	var profile Profile
	if opts.Config != nil {
		p, ok := opts.Config.Profiles[r.Profile]
		// Round 5 Adv-2: error symmetrically when EnvProfile names a
		// non-existent profile. Before the fix, only --profile rejected
		// unknown names; URLBOX_PROFILE silently fell through to env /
		// flag credentials, leaking the wrong profile's behaviour.
		if !ok && opts.FlagProfile != "" {
			return nil, output.NewCLIError(
				output.ErrNotFound,
				`Profile "`+opts.FlagProfile+`" does not exist`,
				"Run 'urlbox config profile list' to see available profiles.",
			)
		}
		if !ok && opts.EnvProfile != "" && opts.FlagProfile == "" {
			return nil, output.NewCLIError(
				output.ErrNotFound,
				`Profile "`+opts.EnvProfile+`" does not exist (URLBOX_PROFILE)`,
				"Run 'urlbox config profile list' to see available profiles, or unset URLBOX_PROFILE.",
			)
		}
		profile = p
	}

	switch {
	case opts.FlagAPIKey != "":
		r.APIKey, r.Source.APIKey = opts.FlagAPIKey, "flag"
	case opts.RepoOverlay != nil && opts.RepoOverlay.APIKey != "":
		r.APIKey, r.Source.APIKey = opts.RepoOverlay.APIKey, "repo"
	case profile.APIKey != "":
		r.APIKey, r.Source.APIKey = profile.APIKey, "profile"
	}

	switch {
	case opts.FlagAPISecret != "":
		r.APISecret, r.Source.APISecret = opts.FlagAPISecret, "flag"
	case opts.EnvAPISecret != "":
		r.APISecret, r.Source.APISecret = opts.EnvAPISecret, "env"
	case opts.RepoOverlay != nil && opts.RepoOverlay.APISecret != "":
		r.APISecret, r.Source.APISecret = opts.RepoOverlay.APISecret, "repo"
	case profile.APISecret != "":
		r.APISecret, r.Source.APISecret = profile.APISecret, "profile"
	}

	switch {
	case opts.FlagAPIHost != "":
		r.APIHost, r.Source.APIHost = opts.FlagAPIHost, "flag"
	case opts.EnvAPIHost != "":
		r.APIHost, r.Source.APIHost = opts.EnvAPIHost, "env"
	case opts.RepoOverlay != nil && opts.RepoOverlay.APIHost != "":
		r.APIHost, r.Source.APIHost = opts.RepoOverlay.APIHost, "repo"
	case profile.APIHost != "":
		r.APIHost, r.Source.APIHost = profile.APIHost, "profile"
	default:
		r.APIHost, r.Source.APIHost = DefaultAPIHost, "default"
	}

	return r, nil
}
