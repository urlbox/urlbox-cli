package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newLogoutCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "logout",
		Short: "Sign out and revoke this device's session",
		Long: `Sign out of Urlbox on this machine.

Revokes only this device's session server-side (your dashboard and other
devices stay signed in) and clears the stored session, active organisation,
active project, and render credential. If the server is unreachable the
local state is cleared anyway.

Examples:
  urlbox logout
  urlbox logout --output-format json`,
		Args: cobra.NoArgs,
		RunE: runLogout,
	}
	attachSessionRetryFlags(c)
	return c
}

func runLogout(cmd *cobra.Command, _ []string) error {
	host, profileName, cliErr := sessionHost(cmd)
	if cliErr != nil {
		return cliErr
	}
	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return cfgErr
	}
	profile := cfg.Profiles[profileName]
	if profile.SessionToken == "" {
		env := output.NewEnvelope("logout", map[string]any{"logged_out": false}, "Not logged in.", nil)
		return writeEnvelope(cmd, env)
	}

	client := newSessionClient(cmd, host, profile.SessionToken)
	if err := client.PostJSON(context.Background(), "/v1/auth/sign-out", map[string]string{}, nil); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: could not reach the server to revoke the session (%v); clearing local login anyway.\n", err)
	}

	if cliErr := updateProfile(profileName, func(p *config.Profile) {
		p.SessionToken = ""
		p.ActiveOrg = ""
		p.ActiveProject = ""
		p.APIKey = ""
		p.APISecret = ""
	}); cliErr != nil {
		return cliErr
	}

	env := output.NewEnvelope("logout", map[string]any{"logged_out": true}, "Logged out.", nil)
	return writeEnvelope(cmd, env)
}
