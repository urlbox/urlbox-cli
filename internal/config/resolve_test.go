package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestResolve_FlagBeatsEnvBeatsProfile(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "p_pub", APISecret: "p_sec"},
		},
	}

	r, err := config.Resolve(config.ResolveOptions{
		FlagAPISecret: "flag_sec", EnvAPISecret: "env_sec", Config: cfg,
	})
	must(t, err)
	if r.APISecret != "flag_sec" || r.Source.APISecret != "flag" {
		t.Errorf("flag tier: secret=%q source=%q", r.APISecret, r.Source.APISecret)
	}

	r, err = config.Resolve(config.ResolveOptions{EnvAPISecret: "env_sec", Config: cfg})
	must(t, err)
	if r.APISecret != "env_sec" || r.Source.APISecret != "env" {
		t.Errorf("env tier: secret=%q source=%q", r.APISecret, r.Source.APISecret)
	}

	r, err = config.Resolve(config.ResolveOptions{Config: cfg})
	must(t, err)
	if r.APISecret != "p_sec" || r.Source.APISecret != "profile" {
		t.Errorf("profile tier: secret=%q source=%q", r.APISecret, r.Source.APISecret)
	}
}

func TestResolve_RepoOverlayBeatsProfile(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APISecret: "p_sec"}},
	}
	r, err := config.Resolve(config.ResolveOptions{
		Config: cfg, RepoOverlay: &config.RepoOverlay{APISecret: "repo_sec"},
	})
	must(t, err)
	if r.APISecret != "repo_sec" || r.Source.APISecret != "repo" {
		t.Errorf("repo tier: secret=%q source=%q", r.APISecret, r.Source.APISecret)
	}
}

func TestResolve_EnvBeatsRepo(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APISecret: "p_sec"}},
	}
	r, err := config.Resolve(config.ResolveOptions{
		Config: cfg, EnvAPISecret: "env_sec",
		RepoOverlay: &config.RepoOverlay{APISecret: "repo_sec"},
	})
	must(t, err)
	if r.APISecret != "env_sec" {
		t.Errorf("env > repo: got %q", r.APISecret)
	}
}

func TestResolve_ProfileFlagSelectsProfile(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIKey: "p_default"},
			"staging": {APIKey: "p_staging"},
		},
	}
	r, err := config.Resolve(config.ResolveOptions{FlagProfile: "staging", Config: cfg})
	must(t, err)
	if r.Profile != "staging" || r.APIKey != "p_staging" {
		t.Errorf("Profile=%q APIKey=%q", r.Profile, r.APIKey)
	}
}

func TestResolve_UnknownProfileFlag_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIKey: "x"}},
	}
	_, err := config.Resolve(config.ResolveOptions{FlagProfile: "ghost", Config: cfg})
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error is %T, want *output.CLIError", err)
	}
	if cliErr.Code != output.ErrNotFound {
		t.Errorf("Code=%v want ErrNotFound", cliErr.Code)
	}
	if cliErr.Message != `Profile "ghost" does not exist` {
		t.Errorf("Message=%q", cliErr.Message)
	}
	if cliErr.Hint != "Run 'urlbox config profile list' to see available profiles." {
		t.Errorf("Hint=%q", cliErr.Hint)
	}
}

func TestResolve_DefaultAPIHost_WhenNothingSet(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIKey: "x"}},
	}
	r, err := config.Resolve(config.ResolveOptions{Config: cfg})
	must(t, err)
	if r.APIHost != config.DefaultAPIHost || r.Source.APIHost != "default" {
		t.Errorf("APIHost=%q Source=%q", r.APIHost, r.Source.APIHost)
	}
}

func TestResolve_NilConfig_OK(t *testing.T) {
	r, err := config.Resolve(config.ResolveOptions{FlagAPISecret: "s"})
	must(t, err)
	if r.APISecret != "s" || r.APIHost != config.DefaultAPIHost {
		t.Errorf("got APISecret=%q APIHost=%q", r.APISecret, r.APIHost)
	}
}

func TestResolve_EnvProfile_PicksProfile(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"work": {APIKey: "p_work"},
		},
	}
	r, err := config.Resolve(config.ResolveOptions{EnvProfile: "work", Config: cfg})
	must(t, err)
	if r.Profile != "work" || r.Source.Profile != "env" {
		t.Errorf("Profile=%q Source=%q", r.Profile, r.Source.Profile)
	}
	if r.APIKey != "p_work" {
		t.Errorf("APIKey=%q", r.APIKey)
	}
}

// TestResolve_UnknownEnvProfile_Errors pins Round 5 Adv-2: when
// URLBOX_PROFILE names a profile that doesn't exist, Resolve must
// return ErrNotFound rather than silently fall through to env/flag
// credentials on the (empty) Profile struct — that fallthrough
// leaked the wrong profile's behavior. Mirrors the existing
// FlagProfile rejection above.
func TestResolve_UnknownEnvProfile_Errors(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"real": {APIKey: "p_real"},
		},
	}
	_, err := config.Resolve(config.ResolveOptions{EnvProfile: "ghost", Config: cfg})
	if err == nil {
		t.Fatal("expected error for unknown URLBOX_PROFILE")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("expected *CLIError, got %T", err)
	}
	if cli.Code != output.ErrNotFound {
		t.Errorf("code=%v, want not_found", cli.Code)
	}
	if !strings.Contains(cli.Message, "ghost") || !strings.Contains(cli.Message, "URLBOX_PROFILE") {
		t.Errorf("message should name 'ghost' and 'URLBOX_PROFILE'; got %q", cli.Message)
	}
}

// TestResolve_UnknownEnvProfile_FlagBeats pins precedence: if both
// FlagProfile and EnvProfile are set and FlagProfile is valid, the
// env-profile mismatch is not a problem (the flag wins).
func TestResolve_UnknownEnvProfile_FlagBeats(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"real": {APIKey: "p_real"},
		},
	}
	r, err := config.Resolve(config.ResolveOptions{
		FlagProfile: "real",
		EnvProfile:  "ghost",
		Config:      cfg,
	})
	must(t, err)
	if r.Profile != "real" || r.Source.Profile != "flag" {
		t.Errorf("Profile=%q Source=%q (flag should win)", r.Profile, r.Source.Profile)
	}
}

func TestResolve_RepoOverlayProfileWins_OverEnvProfile(t *testing.T) {
	cfg := &config.Config{
		Profiles: map[string]config.Profile{
			"repo": {APIKey: "p_repo"},
			"env":  {APIKey: "p_env"},
		},
	}
	r, err := config.Resolve(config.ResolveOptions{
		EnvProfile:  "env",
		RepoOverlay: &config.RepoOverlay{Profile: "repo"},
		Config:      cfg,
	})
	must(t, err)
	if r.Profile != "repo" || r.Source.Profile != "repo" {
		t.Errorf("Profile=%q Source=%q", r.Profile, r.Source.Profile)
	}
}

func TestResolve_APIHost_FlagBeatsEnvBeatsRepoBeatsProfile(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIHost: "https://profile.example"}},
	}

	r, err := config.Resolve(config.ResolveOptions{
		FlagAPIHost: "https://flag.example",
		EnvAPIHost:  "https://env.example",
		RepoOverlay: &config.RepoOverlay{APIHost: "https://repo.example"},
		Config:      cfg,
	})
	must(t, err)
	if r.APIHost != "https://flag.example" || r.Source.APIHost != "flag" {
		t.Errorf("flag tier: %q (%q)", r.APIHost, r.Source.APIHost)
	}

	r, err = config.Resolve(config.ResolveOptions{
		EnvAPIHost:  "https://env.example",
		RepoOverlay: &config.RepoOverlay{APIHost: "https://repo.example"},
		Config:      cfg,
	})
	must(t, err)
	if r.APIHost != "https://env.example" || r.Source.APIHost != "env" {
		t.Errorf("env tier: %q (%q)", r.APIHost, r.Source.APIHost)
	}

	r, err = config.Resolve(config.ResolveOptions{
		RepoOverlay: &config.RepoOverlay{APIHost: "https://repo.example"},
		Config:      cfg,
	})
	must(t, err)
	if r.APIHost != "https://repo.example" || r.Source.APIHost != "repo" {
		t.Errorf("repo tier: %q (%q)", r.APIHost, r.Source.APIHost)
	}

	r, err = config.Resolve(config.ResolveOptions{Config: cfg})
	must(t, err)
	if r.APIHost != "https://profile.example" || r.Source.APIHost != "profile" {
		t.Errorf("profile tier: %q (%q)", r.APIHost, r.Source.APIHost)
	}
}

func TestResolve_APIKey_FromRepoOverlay(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIKey: "p_default"}},
	}
	r, err := config.Resolve(config.ResolveOptions{
		RepoOverlay: &config.RepoOverlay{APIKey: "repo_pub"},
		Config:      cfg,
	})
	must(t, err)
	if r.APIKey != "repo_pub" || r.Source.APIKey != "repo" {
		t.Errorf("APIKey=%q Source=%q", r.APIKey, r.Source.APIKey)
	}
}

// TestResolve_EnvAPISecret_InvalidUTF8_Rejected pins Round 8 FF:
// URLBOX_API_SECRET used to bypass ValidateSecretValue entirely. The
// adversarial demo: signing HMACs with control-char-corrupted env
// bytes. Resolve now applies the same gate the flag/stdin/file paths
// already enforce.
func TestResolve_EnvAPISecret_InvalidUTF8_Rejected(t *testing.T) {
	_, err := config.Resolve(config.ResolveOptions{
		EnvAPISecret: "sec_aaa\xed\xa0\x80bbb", // lone surrogate
	})
	if err == nil {
		t.Fatal("invalid UTF-8 env secret should error")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cli.Code != output.ErrUsage {
		t.Errorf("code=%q, want usage", cli.Code)
	}
	if !strings.Contains(cli.Message, "URLBOX_API_SECRET") {
		t.Errorf("message should name the env var; got %q", cli.Message)
	}
}

// TestResolve_EnvAPISecret_ControlChar_Rejected — covers the H1
// adversarial repro directly: env secret with \x01 was accepted.
func TestResolve_EnvAPISecret_ControlChar_Rejected(t *testing.T) {
	_, err := config.Resolve(config.ResolveOptions{
		EnvAPISecret: "sec_aaa\x01bbb",
	})
	if err == nil {
		t.Fatal("env secret with control char should error")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
		t.Errorf("expected ErrUsage; got %v", err)
	}
}

// TestResolve_EnvAPISecret_ZWJ_Rejected — Cf-category char in env path.
func TestResolve_EnvAPISecret_ZWJ_Rejected(t *testing.T) {
	_, err := config.Resolve(config.ResolveOptions{
		EnvAPISecret: "sec_aaa\u200dbbb", // zero-width joiner
	})
	if err == nil {
		t.Fatal("env secret with ZWJ should error")
	}
}

// TestResolve_EnvAPISecret_CombiningMark_Rejected — Mn-category char in env path.
func TestResolve_EnvAPISecret_CombiningMark_Rejected(t *testing.T) {
	_, err := config.Resolve(config.ResolveOptions{
		EnvAPISecret: "sec_aaa\u0300bbb", // combining grave
	})
	if err == nil {
		t.Fatal("env secret with combining mark should error")
	}
}

// TestResolve_EnvAPISecret_Trimmed pins paste-safety: leading/trailing
// whitespace (incl. Unicode space cats) is trimmed, not rejected.
func TestResolve_EnvAPISecret_Trimmed(t *testing.T) {
	r, err := config.Resolve(config.ResolveOptions{
		EnvAPISecret: "  sec_clean_abc  \n",
	})
	if err != nil {
		t.Fatalf("clean env secret with surrounding whitespace should succeed: %v", err)
	}
	if r.APISecret != "sec_clean_abc" {
		t.Errorf("APISecret=%q, want sec_clean_abc (trimmed)", r.APISecret)
	}
	if r.Source.APISecret != "env" {
		t.Errorf("Source.APISecret=%q, want env", r.Source.APISecret)
	}
}

// ─── Class 1 (v1.0.4) ─────────────────────────────────────────────
// Invariant: every credential/host value reaching the API client has
// been through ValidateSecretValue / ValidateAPIHost, regardless of
// source. Round 8 FF+GG closed env + flag paths but the overlay,
// FlagAPISecret, and profile-from-disk paths were unvalidated.
//
// These tests deliberately use cases that fail under the EXISTING
// validators — the v1.0.4 change is purely "make Resolve call those
// validators on more inputs." The "plain http to remote host" case
// is in apihost_validate_test.go because that's a tightening of the
// validator itself.

// TestResolve_HostileRepoOverlay_APIHost_Rejected pins the overlay
// path through ValidateAPIHost. Each case is already rejected by
// ValidateAPIHost when called via env/flag — the v1.0.4 fix is that
// overlay-supplied values now flow through the same gate.
func TestResolve_HostileRepoOverlay_APIHost_Rejected(t *testing.T) {
	cases := []struct {
		name, host string
	}{
		{"javascript scheme", "javascript:alert(1)"},
		{"file scheme", "file:///etc/passwd"},
		{"ftp scheme", "ftp://evil.example"},
		{"embedded credentials", "https://u:p@evil.example"},
		{"CRLF injection", "https://api.urlbox.com\r\nX-Evil: 1"},
		{"control char in path", "https://api.urlbox.com\x00x"},
		{"empty after trim", "   "},
		{"query string", "https://api.urlbox.com/?evil=1"},
		{"fragment", "https://api.urlbox.com/#evil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Resolve(config.ResolveOptions{
				RepoOverlay: &config.RepoOverlay{APIHost: tc.host},
			})
			if err == nil {
				t.Fatalf("expected overlay api_host %q to be rejected", tc.host)
			}
			var cli *output.CLIError
			if !errors.As(err, &cli) {
				t.Fatalf("expected *output.CLIError, got %T", err)
			}
			if cli.Code != output.ErrUsage {
				t.Errorf("expected ErrUsage, got %v", cli.Code)
			}
			// Error must identify the overlay file as the source so the
			// user knows where the bad value came from.
			if !strings.Contains(cli.Message, ".urlbox/config.json") {
				t.Errorf("expected error to name overlay source; got %q", cli.Message)
			}
		})
	}
}

// TestResolve_HostileRepoOverlay_APISecret_Rejected pins the overlay
// path through ValidateSecretValue. Cases mirror the existing
// EnvAPISecret rejection suite.
func TestResolve_HostileRepoOverlay_APISecret_Rejected(t *testing.T) {
	cases := []struct {
		name, secret string
	}{
		{"control char", "ubx_sk_abc\x07def"},
		{"zero-width joiner", "ubx_sk_abc\u200ddef"},
		{"combining mark", "ubx_sk_abc\u0300def"},
		{"empty after trim", "   "},
		{"invalid utf-8", "ubx_sk_\xff\xfe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Resolve(config.ResolveOptions{
				RepoOverlay: &config.RepoOverlay{APISecret: tc.secret},
			})
			if err == nil {
				t.Fatalf("expected overlay api_secret to be rejected")
			}
			var cli *output.CLIError
			if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
				t.Errorf("expected ErrUsage, got %v", err)
			}
			if !strings.Contains(cli.Message, ".urlbox/config.json") {
				t.Errorf("expected error to name overlay source; got %q", cli.Message)
			}
		})
	}
}

// TestResolve_FlagAPISecret_Validated pins the --api-secret path
// through ValidateSecretValue. Upstream auth.go validates on write,
// but the read path (Resolve consuming FlagAPISecret) was a leak —
// the chokepoint discipline says Resolve is the single gate.
func TestResolve_FlagAPISecret_Validated(t *testing.T) {
	_, err := config.Resolve(config.ResolveOptions{
		FlagAPISecret: "ubx_sk_evil\x07char",
	})
	if err == nil {
		t.Fatal("expected --api-secret with control char to be rejected")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
		t.Errorf("expected ErrUsage, got %v", err)
	}
	if !strings.Contains(cli.Message, "--api-secret") {
		t.Errorf("expected error to name --api-secret as source; got %q", cli.Message)
	}
}

// TestResolve_HostileProfileFromDisk_APIHost_Rejected pins defense-
// in-depth: a manually-edited ~/.config/urlbox/config.json with a
// hostile api_host on a profile must not silently work. The
// write-side validators are bypassed by direct edits; Resolve adds
// the read-side check.
func TestResolve_HostileProfileFromDisk_APIHost_Rejected(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APIHost: "javascript:alert(1)", APIKey: "x"},
		},
	}
	_, err := config.Resolve(config.ResolveOptions{Config: cfg})
	if err == nil {
		t.Fatal("expected hostile profile api_host loaded from disk to be rejected")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
		t.Errorf("expected ErrUsage, got %v", err)
	}
	if !strings.Contains(cli.Message, `profile "default"`) {
		t.Errorf("expected error to name profile source; got %q", cli.Message)
	}
}

// TestResolve_HostileProfileFromDisk_APISecret_Rejected — same
// defense-in-depth for the secret field on disk-loaded profiles.
func TestResolve_HostileProfileFromDisk_APISecret_Rejected(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {APISecret: "ubx_sk_evil\x07char"},
		},
	}
	_, err := config.Resolve(config.ResolveOptions{Config: cfg})
	if err == nil {
		t.Fatal("expected hostile profile api_secret loaded from disk to be rejected")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
		t.Errorf("expected ErrUsage, got %v", err)
	}
}

// TestResolve_CleanProfileFromDisk_Accepted pins the negative: clean
// profile values still resolve successfully (the validate-on-read
// pass must not reject legitimate values).
func TestResolve_CleanProfileFromDisk_Accepted(t *testing.T) {
	cfg := &config.Config{
		DefaultProfile: "default",
		Profiles: map[string]config.Profile{
			"default": {
				APIKey:    "ubx_pk_real",
				APISecret: "ubx_sk_real",
				APIHost:   "https://api.urlbox.com",
			},
		},
	}
	r, err := config.Resolve(config.ResolveOptions{Config: cfg})
	if err != nil {
		t.Fatalf("clean profile should resolve cleanly: %v", err)
	}
	if r.APIHost != "https://api.urlbox.com" || r.APISecret != "ubx_sk_real" {
		t.Errorf("unexpected resolve result: %+v", r)
	}
}
