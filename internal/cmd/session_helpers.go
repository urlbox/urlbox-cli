package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/prompt"
)

type sessionState struct {
	Host        string
	ProfileName string
	Profile     config.Profile
	Client      *api.SessionClient
}

// attachSessionRetryFlags registers --no-retry and --max-retries as persistent
// flags on a top-level session command so every subcommand inherits them. Names,
// help text, and defaults are byte-identical to the render/status surface; the
// values are consumed centrally in newSessionClient.
func attachSessionRetryFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("no-retry", false, "Disable automatic retries on 429 / 5xx")
	cmd.PersistentFlags().Int("max-retries", api.DefaultRetryConfig().MaxRetries, "Maximum retry attempts on 429 / 5xx")
}

// sessionRetryConfig reads the two retry flags off cmd (inherited from the
// top-level session command) and builds the matching RetryConfig. --no-retry
// wins over --max-retries, matching the render/status consumption semantics.
func sessionRetryConfig(cmd *cobra.Command) api.RetryConfig {
	noRetry, _ := cmd.Flags().GetBool("no-retry")
	if noRetry {
		return api.NoRetryConfig()
	}
	cfg := api.DefaultRetryConfig()
	if maxRetries, err := cmd.Flags().GetInt("max-retries"); err == nil {
		cfg.MaxRetries = maxRetries
	}
	return cfg
}

// newSessionClient is the one construction site for session clients: it wires
// the retry policy from cmd's flags into the client. All session commands route
// through here (directly or via loadSession) so the flags take effect uniformly.
func newSessionClient(cmd *cobra.Command, host, token string) *api.SessionClient {
	client := api.NewSessionClient(host, token)
	client.SetRetryConfig(sessionRetryConfig(cmd))
	return client
}

func sessionHost(cmd *cobra.Command) (host, profileName string, cliErr *output.CLIError) {
	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return "", "", cfgErr
	}
	flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
	overlay, ovErr := loadRepoOverlay()
	if ovErr != nil {
		return "", "", ovErr
	}
	resolved, rerr := config.Resolve(config.ResolveOptions{
		FlagProfile:  flagProfile,
		EnvAPISecret: os.Getenv(config.EnvAPISecret),
		EnvAPIHost:   os.Getenv(config.EnvAPIHost),
		EnvProfile:   os.Getenv(config.EnvProfile),
		RepoOverlay:  overlay,
		Config:       cfg,
	})
	if rerr != nil {
		var cli *output.CLIError
		if errors.As(rerr, &cli) {
			return "", "", cli
		}
		return "", "", output.NewCLIError(output.ErrUsage, rerr.Error(), "Run `urlbox config path` to locate the config file.")
	}
	host = resolved.APIHost
	if host == "" {
		host = api.ResolveAPIHost()
	}
	return host, resolved.Profile, nil
}

func loadSession(cmd *cobra.Command) (*sessionState, *output.CLIError) {
	host, profileName, cliErr := sessionHost(cmd)
	if cliErr != nil {
		return nil, cliErr
	}
	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return nil, cfgErr
	}
	profile := cfg.Profiles[profileName]
	if profile.SessionToken == "" {
		return nil, output.NewCLIError(
			output.ErrAuth,
			notLoggedInMsg,
			loginHint,
		)
	}
	return &sessionState{
		Host:        host,
		ProfileName: profileName,
		Profile:     profile,
		Client:      newSessionClient(cmd, host, profile.SessionToken),
	}, nil
}

func updateProfile(profileName string, mutate func(*config.Profile)) *output.CLIError {
	err := config.Update(func(c *config.Config) error {
		p := c.Profiles[profileName]
		mutate(&p)
		c.Profiles[profileName] = p
		if c.DefaultProfile == "" {
			c.DefaultProfile = profileName
		}
		return nil
	})
	if err == nil {
		return nil
	}
	var cli *output.CLIError
	if errors.As(err, &cli) {
		return cli
	}
	return output.NewCLIError(output.ErrForbidden, "could not write config: "+err.Error(),
		"Check the permissions of the config directory (`urlbox config path`).")
}

func requireActiveOrg(sess *sessionState) (string, *output.CLIError) {
	if sess.Profile.ActiveOrg == "" {
		return "", output.NewCLIError(output.ErrUsage,
			"no active organisation",
			"Select one with `urlbox orgs select`.")
	}
	return sess.Profile.ActiveOrg, nil
}

func promptPick(label string, options []string, active int) (int, error) {
	idx, err := prompt.SelectOne(label, options, active)
	if errors.Is(err, prompt.ErrNotInteractive) {
		return -1, errNotInteractivePick
	}
	return idx, err
}
