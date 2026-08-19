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

// ProfileName resolves the active profile name from the priority chain:
// flag → repo overlay → env → default_profile → "default".
func ProfileName(flagProfile, envProfile string, overlay *RepoOverlay, cfg *Config) string {
	switch {
	case flagProfile != "":
		return flagProfile
	case overlay != nil && overlay.Profile != "":
		return overlay.Profile
	case envProfile != "":
		return envProfile
	case cfg != nil && cfg.DefaultProfile != "":
		return cfg.DefaultProfile
	default:
		return "default"
	}
}

// Resolve flattens opts into a single Resolved.
//
// Resolve is the SINGLE chokepoint where credential and host values
// from EVERY source are validated — flag, env, repo overlay, and
// profile-loaded-from-disk. Callers can hand any input verbatim;
// Resolve guarantees that any non-empty value reaching the caller has
// passed through ValidateSecretValue / ValidateAPIHost.
//
// Errors:
//   - FlagProfile or EnvProfile names a profile that doesn't exist in
//     opts.Config (Round 5 Adv-2 + Round 7 EE).
//   - Any credential/host value (flag/env/overlay/profile) fails its
//     validator. v1.0.4 Class 1 closed three gaps in Round 8's FF+GG:
//     RepoOverlay.APIHost/APISecret were consumed verbatim, FlagAPISecret
//     relied on upstream auth-command validation only, and profile values
//     loaded from disk bypassed every write-time gate.
//
//nolint:gocritic // ResolveOptions is intentionally passed by value: this is a pure resolver and the input is immutable. A pointer would invite callers to mutate.
func Resolve(opts ResolveOptions) (*Resolved, error) {
	// ─── Validate inputs at the boundary ──────────────────────────
	// Every non-empty external input passes through its validator. Errors
	// are re-framed so the user sees which source supplied the bad value
	// (helpful when a teammate-committed .urlbox/config.json carries a
	// hostile value the user didn't write).
	if opts.EnvAPISecret != "" {
		cleaned, vErr := ValidateSecretValue(opts.EnvAPISecret)
		if vErr != nil {
			vErr.Message = "URLBOX_API_SECRET: " + vErr.Message
			return nil, vErr
		}
		opts.EnvAPISecret = cleaned
	}
	if opts.FlagAPISecret != "" {
		cleaned, vErr := ValidateSecretValue(opts.FlagAPISecret)
		if vErr != nil {
			vErr.Message = "--api-secret: " + vErr.Message
			return nil, vErr
		}
		opts.FlagAPISecret = cleaned
	}
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
	if opts.RepoOverlay != nil {
		if opts.RepoOverlay.APIHost != "" {
			cleaned, vErr := ValidateAPIHost(opts.RepoOverlay.APIHost)
			if vErr != nil {
				vErr.Message = ".urlbox/config.json api_host: " + vErr.Message
				return nil, vErr
			}
			opts.RepoOverlay.APIHost = cleaned
		}
		if opts.RepoOverlay.APISecret != "" {
			cleaned, vErr := ValidateSecretValue(opts.RepoOverlay.APISecret)
			if vErr != nil {
				vErr.Message = ".urlbox/config.json api_secret: " + vErr.Message
				return nil, vErr
			}
			opts.RepoOverlay.APISecret = cleaned
		}
	}
	r := &Resolved{}

	r.Profile = ProfileName(opts.FlagProfile, opts.EnvProfile, opts.RepoOverlay, opts.Config)
	switch {
	case opts.FlagProfile != "":
		r.Source.Profile = "flag"
	case opts.RepoOverlay != nil && opts.RepoOverlay.Profile != "":
		r.Source.Profile = "repo"
	case opts.EnvProfile != "":
		r.Source.Profile = "env"
	case opts.Config != nil && opts.Config.DefaultProfile != "":
		r.Source.Profile = "default_profile"
	default:
		r.Source.Profile = "default"
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

		// v1.0.4 Class 1 — defense-in-depth: profile values were validated
		// at write time (config set, profile create), but a manually-edited
		// ~/.config/urlbox/config.json bypasses every write-time gate.
		// Validate on read so the read path is the same single chokepoint
		// the rest of Resolve already is.
		if profile.APIHost != "" {
			cleaned, vErr := ValidateAPIHost(profile.APIHost)
			if vErr != nil {
				vErr.Message = `profile "` + r.Profile + `" api_host: ` + vErr.Message
				return nil, vErr
			}
			profile.APIHost = cleaned
		}
		if profile.APISecret != "" {
			cleaned, vErr := ValidateSecretValue(profile.APISecret)
			if vErr != nil {
				vErr.Message = `profile "` + r.Profile + `" api_secret: ` + vErr.Message
				return nil, vErr
			}
			profile.APISecret = cleaned
		}
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
