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
	var apiSecret string
	c := &cobra.Command{
		Use:   "auth",
		Short: "Configure API credentials",
		Long: `Save your Urlbox API secret to the local config file.

Find your API secret in your project's settings on the dashboard:
  https://urlbox.com/dashboard/projects   (open your project → API Secret)

Non-interactive (preferred for agents and CI):
  urlbox auth --api-secret <secret>

Interactive (humans, on a TTY):
  urlbox auth         # prompts once for the secret with masked echo

The env var URLBOX_API_SECRET takes precedence at runtime over the saved value.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret := apiSecret

			interactive := apiSecret == "" && isStdinTTY(cmd.InOrStdin()) && isStderrTTY(cmd.ErrOrStderr())
			if interactive {
				// Prompt label on stderr — keeps stdout clean for --output-format json.
				// Pre-prompt pointer to where the secret lives, so a first-time
				// user doesn't have to hunt or guess the URL.
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Find your API secret in your project's settings: https://urlbox.com/dashboard/projects")
				_, _ = fmt.Fprint(cmd.ErrOrStderr(), "API secret: ")
				s, err := authSecretReader()
				if err != nil {
					return output.NewCLIError(output.ErrUsage, "auth cancelled", err.Error())
				}
				secret = s
			}

			if secret == "" {
				if interactive {
					// User was on a TTY and pressed Enter at the prompt — tell them so,
					// don't suggest "run interactively on a TTY" (they already did).
					return output.NewCLIError(
						output.ErrUsage,
						"empty API secret",
						"Run 'urlbox auth' again and paste your secret at the prompt.",
					)
				}
				return output.NewCLIError(
					output.ErrUsage,
					"missing --api-secret",
					"Pass --api-secret <secret>, export URLBOX_API_SECRET, or run interactively on a TTY. Find your API secret in your project's settings at https://urlbox.com/dashboard/projects.",
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
			p.APISecret = secret
			cfg.Profiles[profileName] = p

			if err := config.Save(cfg); err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"failed to save config",
					err.Error(),
				)
			}

			masked := maskSecret(secret)
			env := output.NewEnvelope(
				"auth",
				map[string]string{
					"masked_secret": masked,
					"profile":       profileName,
					"config_path":   config.Path(),
				},
				fmt.Sprintf("API secret configured (%s)", masked),
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
	c.Flags().StringVar(&apiSecret, "api-secret", "", "Urlbox API secret (skip the interactive prompt — required in CI / non-TTY)")
	return c
}

// maskSecret returns a redacted form of the API secret for safe display.
func maskSecret(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-2:]
}
