package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// stdinTTYOverride forces the TTY result used by auth's interactive gate (test helper).
// Nil = real detection.
var stdinTTYOverride *bool

// SetStdinTTYForTest forces stdin TTY detection for tests.
func SetStdinTTYForTest(v bool) { stdinTTYOverride = &v }

// ResetStdinTTYForTest clears the stdin override.
func ResetStdinTTYForTest() { stdinTTYOverride = nil }

func isStdinTTY(r io.Reader) bool {
	if stdinTTYOverride != nil {
		return *stdinTTYOverride
	}
	if f, ok := r.(*os.File); ok {
		return term.IsTerminal(int(f.Fd())) //nolint:gosec // file descriptors fit in int on every platform Go supports
	}
	return false
}

// AuthSecretReader reads one secret from the user with masked echo.
// Returns the typed value (no trailing newline) and any read error.
type AuthSecretReader func() (string, error)

var authSecretReader AuthSecretReader = defaultAuthSecretReader

// SetAuthSecretReaderForTest injects a stub secret reader.
func SetAuthSecretReaderForTest(f AuthSecretReader) { authSecretReader = f }

// ResetAuthSecretReaderForTest restores the real masked-prompt reader.
func ResetAuthSecretReaderForTest() { authSecretReader = defaultAuthSecretReader }

// defaultAuthSecretReader reads stdin with terminal echo disabled. Caller is
// responsible for printing the prompt label first (so this stays a pure I/O
// primitive that test stubs can replace).
func defaultAuthSecretReader() (string, error) {
	b, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // file descriptors fit in int on every platform Go supports
	if err != nil {
		return "", err
	}
	// Newline after masked input so the next stderr line starts on its own row.
	fmt.Fprintln(os.Stderr)
	return string(b), nil
}

func newAuthCmd() *cobra.Command {
	var apiKey string
	c := &cobra.Command{
		Use:   "auth",
		Short: "Configure API credentials",
		Long: `Save Urlbox API credentials to the local config file.

Non-interactive (preferred for agents and CI):
  urlbox auth --api-key <secret>

Interactive (humans, on a TTY):
  urlbox auth         # prompts once for the secret with masked echo

The env var URLBOX_API_SECRET takes precedence at runtime over the saved value.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret := apiKey

			interactive := apiKey == "" && isStdinTTY(cmd.InOrStdin()) && isStderrTTY(cmd.ErrOrStderr())
			if interactive {
				// Prompt label on stderr — keeps stdout clean for --output-format json.
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "API secret: ")
				s, err := authSecretReader()
				if err != nil {
					return output.NewCLIError(output.ErrUsage, "auth cancelled", err.Error())
				}
				secret = s
			}

			if secret == "" {
				return output.NewCLIError(
					output.ErrUsage,
					"missing --api-key",
					"Pass --api-key <secret>, export URLBOX_API_SECRET, or run interactively on a TTY.",
				)
			}

			cfg, err := config.Load()
			if err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"failed to read config",
					err.Error(),
				)
			}
			if cfg.Profiles == nil {
				cfg.Profiles = map[string]config.Profile{}
			}
			profileName := cfg.DefaultProfile
			if profileName == "" {
				profileName = "default"
				cfg.DefaultProfile = profileName
			}
			p := cfg.Profiles[profileName]
			p.APIKey = secret
			cfg.Profiles[profileName] = p

			if err := config.Save(cfg); err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"failed to save config",
					err.Error(),
				)
			}

			masked := maskKey(secret)
			env := output.NewEnvelope(
				"auth",
				map[string]string{
					"masked_key":  masked,
					"profile":     profileName,
					"config_path": config.Path(),
				},
				fmt.Sprintf("API key configured (%s)", masked),
				[]output.Breadcrumb{
					{Action: "verify", Cmd: "urlbox doctor"},
					{Action: "render", Cmd: "urlbox render <url>"},
				},
			)

			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)
			styles := output.NewStylesForWriter(stdout)

			if jqExpr != "" {
				return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			}
			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	}
	c.Flags().StringVar(&apiKey, "api-key", "", "Urlbox API secret (skip the interactive prompt — required in CI / non-TTY)")
	return c
}

// maskKey returns a redacted form of the key for safe display.
func maskKey(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-2:]
}
