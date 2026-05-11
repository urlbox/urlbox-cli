package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

var supportedConfigKeys = []string{"api_key", "api_secret", "api_host", "default_profile"}

func unknownKeyError(key string) *output.CLIError {
	return output.NewCLIError(
		output.ErrUsage,
		"Unknown config key: "+key,
		"Supported: api_key, api_secret, api_host, default_profile",
	)
}

func newConfigCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "config",
		Short: "Inspect and modify CLI configuration",
		Long:  "Get, set, and inspect the urlbox CLI's persisted configuration.",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	parent.AddCommand(newConfigPathCmd())
	parent.AddCommand(newConfigGetCmd())
	parent.AddCommand(newConfigSetCmd())
	parent.AddCommand(newProfileCmd())
	return parent
}

func newProfileCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "profile",
		Short: "Manage named config profiles",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	parent.AddCommand(newProfileCreateCmd())
	parent.AddCommand(newProfileListCmd())
	parent.AddCommand(newProfileDefaultCmd())
	parent.AddCommand(newProfileDeleteCmd())
	return parent
}

func newProfileCreateCmd() *cobra.Command {
	var apiHost, apiSecret, apiSecretFile, apiKey string
	var apiSecretStdin bool
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			resolvedSecret, cliErr := resolveAPISecretInput(secretStdin, cmd.ErrOrStderr(), apiSecret, cmd.Flags().Changed("api-secret"), apiSecretStdin, apiSecretFile)
			if cliErr != nil {
				return cliErr
			}
			if resolvedSecret == "" {
				resolvedSecret = apiSecret
			}
			cfg, err := config.Load()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
			}
			if _, exists := cfg.Profiles[name]; exists {
				return output.NewCLIError(
					output.ErrConflict,
					`Profile "`+name+`" already exists`,
					"Use 'urlbox config set <key> <value> --profile "+name+"' to update it.",
				)
			}
			if cfg.Profiles == nil {
				cfg.Profiles = map[string]config.Profile{}
			}
			cfg.Profiles[name] = config.Profile{APIKey: apiKey, APISecret: resolvedSecret, APIHost: apiHost}
			if cfg.DefaultProfile == "" {
				cfg.DefaultProfile = name
			}
			if err := config.Save(cfg); err != nil {
				return output.NewCLIError(output.ErrServer, "failed to save config", err.Error())
			}
			env := output.NewEnvelope(
				"config profile create",
				map[string]string{"name": name, "api_host": apiHost},
				`Profile "`+name+`" created`,
				[]output.Breadcrumb{{Action: "use", Cmd: "urlbox --profile " + name + " render <url>"}},
			)
			return writeEnvelope(cmd, env)
		},
	}
	c.Flags().StringVar(&apiHost, "api-host", "", "API host for this profile")
	c.Flags().StringVar(&apiSecret, "api-secret", "", "API secret for this profile (leaks via ps + shell history; prefer --api-secret-stdin or --api-secret-file)")
	c.Flags().BoolVar(&apiSecretStdin, "api-secret-stdin", false, "Read the API secret from stdin until EOF")
	c.Flags().StringVar(&apiSecretFile, "api-secret-file", "", "Read the API secret from the given file (trailing newline trimmed)")
	c.Flags().StringVar(&apiKey, "api-key", "", "Publishable API key for this profile")
	return c
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
			}
			names := make([]string, 0, len(cfg.Profiles))
			for n := range cfg.Profiles {
				names = append(names, n)
			}
			sort.Strings(names)

			rows := make([]map[string]string, 0, len(names))
			for _, n := range names {
				p := cfg.Profiles[n]
				rows = append(rows, map[string]string{
					"name":          n,
					"api_host":      p.APIHost,
					"masked_secret": maskSecret(p.APISecret),
					"is_default":    fmt.Sprintf("%v", n == cfg.DefaultProfile),
				})
			}
			env := output.NewEnvelope(
				"config profile list",
				map[string]any{"profiles": rows, "default": cfg.DefaultProfile},
				fmt.Sprintf("%d profile(s); default = %q", len(rows), cfg.DefaultProfile),
				nil,
			)
			return writeEnvelope(cmd, env)
		},
	}
}

func newProfileDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "default <name>",
		Short: "Set the default profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
			}
			if _, ok := cfg.Profiles[name]; !ok {
				return output.NewCLIError(
					output.ErrNotFound,
					`Profile "`+name+`" does not exist`,
					"Run 'urlbox config profile list' to see available profiles.",
				)
			}
			cfg.DefaultProfile = name
			if err := config.Save(cfg); err != nil {
				return output.NewCLIError(output.ErrServer, "failed to save config", err.Error())
			}
			env := output.NewEnvelope(
				"config profile default",
				map[string]string{"default": name},
				`Default profile set to "`+name+`"`,
				nil,
			)
			return writeEnvelope(cmd, env)
		},
	}
}

func newProfileDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := config.Load()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
			}
			if _, ok := cfg.Profiles[name]; !ok {
				return output.NewCLIError(
					output.ErrNotFound,
					`Profile "`+name+`" does not exist`,
					"Run 'urlbox config profile list' to see available profiles.",
				)
			}
			if name == cfg.DefaultProfile && len(cfg.Profiles) == 1 {
				return output.NewCLIError(
					output.ErrConflict,
					`Cannot delete the only profile "`+name+`"`,
					"Create another profile first, or run 'urlbox auth' to start fresh.",
				)
			}
			if name == cfg.DefaultProfile {
				return output.NewCLIError(
					output.ErrConflict,
					`Cannot delete the default profile "`+name+`"`,
					"Run 'urlbox config profile default <other>' to switch the default first.",
				)
			}
			delete(cfg.Profiles, name)
			if err := config.Save(cfg); err != nil {
				return output.NewCLIError(output.ErrServer, "failed to save config", err.Error())
			}
			env := output.NewEnvelope(
				"config profile delete",
				map[string]string{"deleted": name},
				`Profile "`+name+`" deleted`,
				nil,
			)
			return writeEnvelope(cmd, env)
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the resolved config file path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := output.NewEnvelope(
				"config path",
				map[string]string{"path": config.Path()},
				"Config file path: "+config.Path(),
				nil,
			)
			return writeEnvelopeWithQuietData(cmd, env, config.Path())
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	var reveal bool
	c := &cobra.Command{
		Use:   "get <key>",
		Short: "Read a config value",
		Long: `Read a config value from the resolved profile.

For api_secret, the raw value is masked by default (Round 1 UX I1) to
avoid leaking into scrollback / clipboard / log capture. Pass --reveal
to print the unmasked secret (intended for clipboard-copy workflows
with eyes on the screen).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !isSupportedKey(key) {
				return unknownKeyError(key)
			}
			c, err := config.Load()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
			}
			if key == "default_profile" {
				value := c.DefaultProfile
				env := output.NewEnvelope(
					"config get",
					map[string]string{"key": key, "value": value},
					fmt.Sprintf("%s = %q", key, value),
					nil,
				)
				return writeEnvelopeWithQuietData(cmd, env, value)
			}
			profileName, perr := resolveTargetProfile(cmd, c)
			if perr != nil {
				return perr
			}
			value := readKey(c, profileName, key)
			display := value
			if key == "api_secret" && !reveal && value != "" {
				display = maskSecret(value)
			}
			env := output.NewEnvelope(
				"config get",
				map[string]string{"key": key, "value": display, "profile": profileName},
				fmt.Sprintf("%s = %q", key, display),
				nil,
			)
			return writeEnvelopeWithQuietData(cmd, env, display)
		},
	}
	c.Flags().BoolVar(&reveal, "reveal", false, "Print api_secret unmasked (default: masked)")
	return c
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value on the target profile",
		Long: `Set a config value.

For per-profile keys (api_key, api_secret, api_host) the target profile is:
  - the only profile, if exactly one is configured (no --profile required);
  - the value of --profile, if given (must already exist);
  - otherwise an error: with 0 profiles, run urlbox auth first;
    with 2+, --profile is required.

The default_profile key is top-level and always writes regardless of
profile count.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, val := args[0], args[1]
			if !isSupportedKey(key) {
				return unknownKeyError(key)
			}
			c, err := config.Load()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
			}
			// default_profile is top-level (no profile-target resolution), but the
			// named profile MUST exist — silently accepting a dangling default_profile
			// would break credential resolution downstream.
			if key == "default_profile" {
				if len(c.Profiles) == 0 {
					return output.NewCLIError(
						output.ErrUsage,
						"No profiles configured",
						"Run `urlbox auth --api-secret <secret>` to create one.",
					)
				}
				if _, ok := c.Profiles[val]; !ok {
					return output.NewCLIError(
						output.ErrUsage,
						"Unknown profile: "+val,
						"Configured profiles: "+quotedSortedProfileNames(c.Profiles)+".",
					)
				}
				c.DefaultProfile = val
				if err := config.Save(c); err != nil {
					return output.NewCLIError(output.ErrServer, "failed to save config", err.Error())
				}
				env := output.NewEnvelope(
					"config set",
					map[string]string{"key": key, "value": val},
					fmt.Sprintf("default_profile set to %q", val),
					nil,
				)
				return writeEnvelope(cmd, env)
			}
			profileName, perr := resolveTargetProfile(cmd, c)
			if perr != nil {
				return perr
			}
			writeKey(c, profileName, key, val)
			if err := config.Save(c); err != nil {
				return output.NewCLIError(output.ErrServer, "failed to save config", err.Error())
			}
			// Round 4 M4: mirror the masking that `config get api_secret`
			// already does — never echo a freshly-set secret back through
			// the envelope (CI logs, scrollback, --output-format quiet pipes).
			// The raw value is still persisted on disk; only the
			// human/agent-facing echo is masked.
			displayVal := val
			if key == "api_secret" && val != "" {
				displayVal = maskSecret(val)
			}
			env := output.NewEnvelope(
				"config set",
				map[string]string{"key": key, "value": displayVal, "profile": profileName},
				fmt.Sprintf("%s set in profile %q", key, profileName),
				[]output.Breadcrumb{{Action: "verify", Cmd: "urlbox config get " + key}},
			)
			return writeEnvelope(cmd, env)
		},
	}
}

// resolveTargetProfile picks which profile a per-profile config key acts on,
// per resolved Open Question 4 (smart write).
//
//   - 0 profiles → ErrUsage "No profiles configured"
//   - --profile given → must exist, else ErrUsage "Unknown profile: <name>"
//   - 1 profile, no --profile → that profile (implicit)
//   - 2+ profiles, no --profile → ErrUsage "--profile is required" with sorted name list
func resolveTargetProfile(cmd *cobra.Command, c *config.Config) (string, error) {
	if len(c.Profiles) == 0 {
		return "", output.NewCLIError(
			output.ErrUsage,
			"No profiles configured",
			"Run `urlbox auth --api-secret <secret>` to create one.",
		)
	}
	flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
	if flagProfile != "" {
		if _, ok := c.Profiles[flagProfile]; !ok {
			return "", output.NewCLIError(
				output.ErrUsage,
				"Unknown profile: "+flagProfile,
				"Configured profiles: "+quotedSortedProfileNames(c.Profiles)+".",
			)
		}
		return flagProfile, nil
	}
	if len(c.Profiles) == 1 {
		for name := range c.Profiles {
			return name, nil
		}
	}
	return "", output.NewCLIError(
		output.ErrUsage,
		"--profile is required",
		"Configured profiles: "+quotedSortedProfileNames(c.Profiles)+". Add --profile <name> to choose one.",
	)
}

// quotedSortedProfileNames returns `"a", "b", "c"` — names sorted alphabetically and
// double-quoted so the error reads cleanly. Used in resolveTargetProfile hint text.
func quotedSortedProfileNames(profiles map[string]config.Profile) string {
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = strconv.Quote(n)
	}
	return strings.Join(out, ", ")
}

func isSupportedKey(k string) bool {
	for _, s := range supportedConfigKeys {
		if k == s {
			return true
		}
	}
	return false
}

func readKey(c *config.Config, profile, key string) string {
	p := c.Profiles[profile]
	switch key {
	case "api_key":
		return p.APIKey
	case "api_secret":
		return p.APISecret
	case "api_host":
		return p.APIHost
	}
	return ""
}

// writeKey mutates an existing profile. Callers must have validated that
// `profile` exists in `c.Profiles` (resolveTargetProfile does this for
// per-profile keys; default_profile is handled by the RunE before writeKey
// is called).
func writeKey(c *config.Config, profile, key, val string) {
	p := c.Profiles[profile]
	switch key {
	case "api_key":
		p.APIKey = val
	case "api_secret":
		p.APISecret = val
	case "api_host":
		p.APIHost = val
	}
	c.Profiles[profile] = p
}

// writeEnvelope writes the success envelope using the resolved formatter.
func writeEnvelope(cmd *cobra.Command, env *output.Envelope) error {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	stdout := cmd.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if jqExpr != "" {
		return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
	}
	styles := output.NewStylesForWriter(stdout)
	return output.NewFormatter(format, styles).WriteSuccess(stdout, env)
}

// writeEnvelopeWithQuietData lets `config get` / `config path` emit just the
// scalar value (JSON-quoted) when --output-format quiet, while still using the
// envelope otherwise.
func writeEnvelopeWithQuietData(cmd *cobra.Command, env *output.Envelope, scalar string) error {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	stdout := cmd.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if format == output.FormatQuiet && jqExpr == "" {
		return jsonScalarLine(stdout, scalar)
	}
	return writeEnvelope(cmd, env)
}

// jsonScalarLine writes a single JSON-encoded string followed by a newline.
// Matches the QuietFormatter convention: scalar data → bare JSON value on its own line.
func jsonScalarLine(w io.Writer, s string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}
