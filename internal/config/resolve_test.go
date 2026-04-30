package config_test

import (
	"errors"
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
