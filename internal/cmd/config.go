package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

var supportedConfigKeys = []string{"api_key", "api_secret", "api_host", "default_profile", "session_token", "active_org", "active_project"}

// profileNameRE pins the allowed shape of a profile name: must start with
// an alphanumeric, then 0–63 more alphanumerics / underscore / hyphen,
// totalling 1–64 chars. Round 5 Adv-4: profile names previously accepted
// path separators (`/`, `..`), control chars (\n, \r, \t), null bytes
// (silent truncation collisions: a\0b vs a), leading whitespace or dots,
// and arbitrarily long strings. All footguns in their own way.
//
// The character class is deliberately narrow — profiles are user-facing
// keys that appear in URLs/CLI args/file paths, and we don't need the
// power of Unicode identifiers for what is essentially a label.
var profileNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// validateProfileName returns ErrUsage when name violates profileNameRE.
// Called from `config profile create` before any persistence so an
// invalid name can never reach the config file.
func validateProfileName(name string) *output.CLIError {
	if !profileNameRE.MatchString(name) {
		return output.NewCLIError(
			output.ErrUsage,
			fmt.Sprintf("invalid profile name %q", name),
			"Profile names must be 1–64 characters, start with a letter or digit, and contain only letters, digits, '_', and '-'. Rejected to prevent path traversal, null-byte truncation, control characters, and ambiguous leading characters.",
		)
	}
	return nil
}

func unknownKeyError(key string) *output.CLIError {
	return output.NewCLIError(
		output.ErrUsage,
		"Unknown config key: "+key,
		"Supported: api_key, api_secret, api_host, default_profile, session_token, active_org, active_project",
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
			if cliErr := validateProfileName(name); cliErr != nil {
				return cliErr
			}
			resolvedSecret, cliErr := resolveAPISecretInput(secretStdin, cmd.ErrOrStderr(), apiSecret, cmd.Flags().Changed("api-secret"), apiSecretStdin, apiSecretFile)
			if cliErr != nil {
				return cliErr
			}
			// Validate the resolved secret value when one was provided. Round 6
			// class-fix: profile create must go through the same gate every
			// secret-writing path uses. An empty resolvedSecret here means
			// no flag was passed — that's allowed for profile create (the
			// profile can be created secretless and have the secret added
			// later via auth or config set).
			if resolvedSecret != "" {
				validated, vErr := config.ValidateSecretValue(resolvedSecret)
				if vErr != nil {
					return vErr
				}
				resolvedSecret = validated
			}
			// Round 8 GG: same gate for api_host as for api_secret —
			// validates scheme, rejects embedded creds + CRLF + control
			// chars before persisting.
			if apiHost != "" {
				validated, vErr := config.ValidateAPIHost(apiHost)
				if vErr != nil {
					return vErr
				}
				apiHost = validated
			}
			// Atomic check + create under the config-file lock (Round 7 CC
			// class-fix): the previous Load -> check -> Save sequence raced
			// when parallel `config profile create` calls hit the same
			// XDG_CONFIG_HOME — 20 parallel calls used to lose 5-6.
			if err := config.Update(func(cfg *config.Config) error {
				if _, exists := cfg.Profiles[name]; exists {
					return output.NewCLIError(
						output.ErrConflict,
						`Profile "`+name+`" already exists`,
						"Use 'urlbox config set <key> <value> --profile "+name+"' to update it.",
					)
				}
				cfg.Profiles[name] = config.Profile{APIKey: apiKey, APISecret: resolvedSecret, APIHost: apiHost}
				if cfg.DefaultProfile == "" {
					cfg.DefaultProfile = name
				}
				return nil
			}); err != nil {
				var cli *output.CLIError
				if errors.As(err, &cli) {
					return cli
				}
				return output.NewCLIError(output.ErrServer, "failed to write config", err.Error())
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
			cfg, cfgErr := config.LoadOrCLIError()
			if cfgErr != nil {
				return cfgErr
			}
			names := make([]string, 0, len(cfg.Profiles))
			for n := range cfg.Profiles {
				names = append(names, n)
			}
			sort.Strings(names)

			rows := make([]map[string]any, 0, len(names))
			for _, n := range names {
				p := cfg.Profiles[n]
				rows = append(rows, map[string]any{
					"name":          n,
					"api_host":      p.APIHost,
					"masked_secret": maskSecret(p.APISecret),
					// Round 8 MM: was string("true"/"false") because the
					// row was typed map[string]string. Now bool so JSON
					// consumers (agents) can use it directly without
					// string-comparison.
					"is_default": n == cfg.DefaultProfile,
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
			if err := config.Update(func(cfg *config.Config) error {
				if _, ok := cfg.Profiles[name]; !ok {
					return output.NewCLIError(
						output.ErrNotFound,
						`Profile "`+name+`" does not exist`,
						"Run 'urlbox config profile list' to see available profiles.",
					)
				}
				cfg.DefaultProfile = name
				return nil
			}); err != nil {
				var cli *output.CLIError
				if errors.As(err, &cli) {
					return cli
				}
				return output.NewCLIError(output.ErrServer, "failed to write config", err.Error())
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
			if err := config.Update(func(cfg *config.Config) error {
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
						"Create another profile first, or run `urlbox login` / `urlbox auth --api-secret <secret>` to start fresh.",
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
				return nil
			}); err != nil {
				var cli *output.CLIError
				if errors.As(err, &cli) {
					return cli
				}
				return output.NewCLIError(output.ErrServer, "failed to write config", err.Error())
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

For api_secret and session_token, the raw value is masked by default
(Round 1 UX I1) to avoid leaking into scrollback / clipboard / log
capture. Pass --reveal to print the unmasked value (intended for
clipboard-copy workflows with eyes on the screen).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !isSupportedKey(key) {
				return unknownKeyError(key)
			}
			c, cfgErr := config.LoadOrCLIError()
			if cfgErr != nil {
				return cfgErr
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
			if (key == "api_secret" || key == "session_token") && !reveal && value != "" {
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
	c.Flags().BoolVar(&reveal, "reveal", false, "Print api_secret / session_token unmasked (default: masked)")
	return c
}

func newConfigSetCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a config value on the target profile",
		Long: `Set a config value.

For per-profile keys (api_key, api_secret, api_host) the target profile is:
  - the only profile, if exactly one is configured (no --profile required);
  - the value of --profile, if given (must already exist);
  - otherwise an error: with 0 profiles, run urlbox login first;
    with 2+, --profile is required.

Setting api_secret on a profile that already has a different secret
refuses unless --force is passed.

The default_profile key is top-level and always writes regardless of
profile count.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, val := args[0], args[1]
			if !isSupportedKey(key) {
				return unknownKeyError(key)
			}
			// Validate api_secret / session_token values through the same gate
			// every secret-writing path uses. Rejects empty / whitespace /
			// control chars. Round 6 class-fix.
			if key == "api_secret" || key == "session_token" {
				validated, vErr := config.ValidateSecretValue(val)
				if vErr != nil {
					return vErr
				}
				val = validated
			}
			// Round 8 GG: api_host gets the same treatment — was accepting
			// javascript:, file://, embedded credentials, CRLF before.
			if key == "api_host" {
				validated, vErr := config.ValidateAPIHost(val)
				if vErr != nil {
					return vErr
				}
				val = validated
			}
			// default_profile is top-level (no profile-target resolution), but the
			// named profile MUST exist — silently accepting a dangling default_profile
			// would break credential resolution downstream.
			if key == "default_profile" {
				if err := config.Update(func(c *config.Config) error {
					if len(c.Profiles) == 0 {
						return output.NewCLIError(
							output.ErrUsage,
							"No profiles configured",
							credentialHint,
						)
					}
					if _, ok := c.Profiles[val]; !ok {
						return output.NewCLIError(
							output.ErrNotFound,
							`Profile "`+val+`" does not exist`,
							"Run 'urlbox config profile list' to see available profiles.",
						)
					}
					c.DefaultProfile = val
					return nil
				}); err != nil {
					var cli *output.CLIError
					if errors.As(err, &cli) {
						return cli
					}
					return output.NewCLIError(output.ErrServer, "failed to write config", err.Error())
				}
				env := output.NewEnvelope(
					"config set",
					map[string]string{"key": key, "value": val},
					fmt.Sprintf("default_profile set to %q", val),
					nil,
				)
				return writeEnvelope(cmd, env)
			}

			// Per-profile key (api_key / api_secret / api_host). Profile
			// resolution + overwrite-guard + write all happen under one
			// Update so the read-modify-write window is atomic. Round 7 CC.
			var profileName string
			if err := config.Update(func(c *config.Config) error {
				name, perr := resolveTargetProfile(cmd, c)
				if perr != nil {
					return perr
				}
				profileName = name
				if key == "api_secret" && !force {
					existing := c.Profiles[name].APISecret
					if existing != "" && existing != val {
						return output.NewCLIError(
							output.ErrConflict,
							fmt.Sprintf("profile %q already has an API secret (%s); overwrite refused", name, maskSecret(existing)),
							"Pass --force to overwrite, or use `urlbox config profile create <name>` for a separate profile.",
						)
					}
				}
				writeKey(c, name, key, val)
				return nil
			}); err != nil {
				var cli *output.CLIError
				if errors.As(err, &cli) {
					return cli
				}
				return output.NewCLIError(output.ErrServer, "failed to write config", err.Error())
			}
			// Round 4 M4: mirror the masking that `config get api_secret`
			// already does — never echo a freshly-set secret back through
			// the envelope (CI logs, scrollback, --output-format quiet pipes).
			// The raw value is still persisted on disk; only the
			// human/agent-facing echo is masked.
			displayVal := val
			if (key == "api_secret" || key == "session_token") && val != "" {
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
	c.Flags().BoolVar(&force, "force", false, "Overwrite an existing api_secret without confirmation.")
	return c
}

// resolveTargetProfile picks which profile a per-profile config key acts on,
// per resolved Open Question 4 (smart write).
//
// Precedence (highest first):
//   - 0 profiles → ErrUsage "No profiles configured" (setup issue)
//   - --profile given → must exist, else ErrNotFound (Round 7 EE: every
//     "user named a profile that doesn't exist" site now reports the same
//     envelope as profile delete/default and the unified config.Resolve)
//   - URLBOX_PROFILE set → must exist, else ErrNotFound (same class)
//   - 1 profile, no flag/env → that profile (implicit)
//   - 2+ profiles, default_profile set and exists → default_profile (Round 5
//     CI-2: matches how render/status/link resolve)
//   - 2+ profiles, no default_profile → ErrUsage "--profile is required"
//     (ambiguity — user didn't specify which, not a name lookup miss)
func resolveTargetProfile(cmd *cobra.Command, c *config.Config) (string, error) {
	if len(c.Profiles) == 0 {
		return "", output.NewCLIError(
			output.ErrUsage,
			"No profiles configured",
			credentialHint,
		)
	}
	flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
	if flagProfile != "" {
		if _, ok := c.Profiles[flagProfile]; !ok {
			return "", output.NewCLIError(
				output.ErrNotFound,
				`Profile "`+flagProfile+`" does not exist`,
				"Run 'urlbox config profile list' to see available profiles.",
			)
		}
		return flagProfile, nil
	}
	if envProfile := os.Getenv(config.EnvProfile); envProfile != "" {
		if _, ok := c.Profiles[envProfile]; !ok {
			return "", output.NewCLIError(
				output.ErrNotFound,
				`Profile "`+envProfile+`" does not exist (URLBOX_PROFILE)`,
				"Run 'urlbox config profile list' to see available profiles, or unset URLBOX_PROFILE.",
			)
		}
		return envProfile, nil
	}
	if len(c.Profiles) == 1 {
		for name := range c.Profiles {
			return name, nil
		}
	}
	// 2+ profiles, no flag, no env: fall back to default_profile if set.
	// Round 5 CI-2 — config get/set used to require --profile here, but
	// render/status/link already resolved default_profile transparently,
	// breaking CI scripts that ran `config set api_key X` after creating
	// a second profile.
	if c.DefaultProfile != "" {
		if _, ok := c.Profiles[c.DefaultProfile]; ok {
			return c.DefaultProfile, nil
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
	case "session_token":
		return p.SessionToken
	case "active_org":
		return p.ActiveOrg
	case "active_project":
		return p.ActiveProject
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
	case "session_token":
		p.SessionToken = val
	case "active_org":
		p.ActiveOrg = val
	case "active_project":
		p.ActiveProject = val
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
