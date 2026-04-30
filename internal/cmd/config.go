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
	return parent
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
	return &cobra.Command{
		Use:   "get <key>",
		Short: "Read a config value",
		Args:  cobra.ExactArgs(1),
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
			env := output.NewEnvelope(
				"config get",
				map[string]string{"key": key, "value": value, "profile": profileName},
				fmt.Sprintf("%s = %q", key, value),
				nil,
			)
			return writeEnvelopeWithQuietData(cmd, env, value)
		},
	}
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
			if key == "default_profile" {
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
			env := output.NewEnvelope(
				"config set",
				map[string]string{"key": key, "value": val, "profile": profileName},
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
			"Run `urlbox auth --api-key <secret>` to create one.",
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
