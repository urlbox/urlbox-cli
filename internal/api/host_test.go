package api_test

import (
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func TestResolveAPIHost_Default(t *testing.T) {
	t.Setenv(api.EnvAPIHost, "")
	got := api.ResolveAPIHost()
	if got != api.DefaultAPIHost {
		t.Errorf("ResolveAPIHost()=%q, want %q", got, api.DefaultAPIHost)
	}
}

func TestResolveAPIHost_EnvOverride(t *testing.T) {
	t.Setenv(api.EnvAPIHost, "https://api-staging.urlbox.com")
	if got := api.ResolveAPIHost(); got != "https://api-staging.urlbox.com" {
		t.Errorf("ResolveAPIHost()=%q, want staging", got)
	}
}

func TestDefaultAPIHost_IsProduction(t *testing.T) {
	if api.DefaultAPIHost != "https://api.urlbox.com" {
		t.Errorf("DefaultAPIHost=%q, want https://api.urlbox.com", api.DefaultAPIHost)
	}
}
