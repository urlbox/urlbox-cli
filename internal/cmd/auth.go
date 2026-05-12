package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

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
// responsible for printing the prompt label BEFORE the read AND for printing
// the trailing newline AFTER the read on the cobra writer (so this stays a
// pure I/O primitive that test stubs can replace, and so the newline
// participates in cobra writer plumbing).
func defaultAuthSecretReader() (string, error) {
	b, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // file descriptors fit in int on every platform Go supports
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// AuthConfirmReader reads one line of plain text from the user — used for
// y/N confirmation prompts (overwrite guard). Echoes input; not for
// secrets.
type AuthConfirmReader func() (string, error)

var authConfirmReader AuthConfirmReader = defaultAuthConfirmReader

// SetAuthConfirmReaderForTest injects a stub confirm reader.
func SetAuthConfirmReaderForTest(f AuthConfirmReader) { authConfirmReader = f }

// ResetAuthConfirmReaderForTest restores the default reader.
func ResetAuthConfirmReaderForTest() { authConfirmReader = defaultAuthConfirmReader }

func defaultAuthConfirmReader() (string, error) {
	var line string
	_, err := fmt.Fscanln(os.Stdin, &line)
	return line, err
}

func newAuthCmd() *cobra.Command {
	var apiSecret, apiSecretFile string
	var apiSecretStdin, force bool
	c := &cobra.Command{
		Use:   "auth",
		Short: "Configure API credentials",
		Long: `Save your Urlbox API secret to the local config file.

Find your API secret in your project's settings on the dashboard:
  https://urlbox.com/dashboard/projects   (open your project → API Secret)

Non-interactive (preferred for agents and CI):
  printf %s "$URLBOX_API_SECRET" | urlbox auth --api-secret-stdin
  urlbox auth --api-secret-file ~/.config/urlbox/secret
  urlbox auth --api-secret <secret>     # least safe: visible in ps + shell history

Interactive (humans, on a TTY):
  urlbox auth         # prompts once for the secret with masked echo

The env var URLBOX_API_SECRET takes precedence at runtime over the saved value.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			secret, cliErr := resolveAPISecretInput(secretStdin, cmd.ErrOrStderr(), apiSecret, cmd.Flags().Changed("api-secret"), apiSecretStdin, apiSecretFile)
			if cliErr != nil {
				return cliErr
			}

			interactive := secret == "" && !apiSecretStdin && apiSecretFile == "" && isStdinTTY(cmd.InOrStdin()) && isStderrTTY(cmd.ErrOrStderr())
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
				// Newline after masked input lands on the cobra writer so it
				// participates in test capture + caller redirection.
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
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
					"missing API secret",
					"Pipe via --api-secret-stdin, read from --api-secret-file <path>, pass --api-secret <secret>, export URLBOX_API_SECRET, or run interactively on a TTY. Find your API secret in your project's settings at https://urlbox.com/dashboard/projects.",
				)
			}

			// Validate the resolved secret value through the same gate every
			// secret-writing path uses (auth / config set / profile create).
			// Catches whitespace-only, leading/trailing whitespace artifacts,
			// and embedded control characters. Round 6 class-fix.
			validated, vErr := validateSecretValue(secret)
			if vErr != nil {
				return vErr
			}
			secret = validated

			// Round 7 CC class-fix: the entire read-modify-write happens
			// inside config.Update so the file lock makes it atomic w.r.t.
			// parallel processes. The TTY prompt logic stays outside the
			// mutate fn (prompting under a lock would deadlock other
			// processes for the prompt's lifetime); on conflict, prompt
			// then retry the Update with force=true.
			flagProfile, _ := cmd.Root().PersistentFlags().GetString("profile")
			envProfile := os.Getenv(config.EnvProfile)

			var profileName string
			runUpdate := func(allowOverwrite bool) error {
				return config.Update(func(cfg *config.Config) error {
					// Round 4 C1: honor --profile and URLBOX_PROFILE. Before this,
					// auth always wrote to cfg.DefaultProfile, silently dropping
					// any --profile flag, which turned the overwrite guard into
					// the 2026-05-08-incident-class clobber when callers
					// intended a non-default target. Precedence: flag > env >
					// cfg.DefaultProfile > "default".
					switch {
					case flagProfile != "":
						if _, ok := cfg.Profiles[flagProfile]; !ok {
							return output.NewCLIError(
								output.ErrUsage,
								"Unknown profile: "+flagProfile,
								"Configured profiles: "+quotedSortedProfileNames(cfg.Profiles)+". Use `urlbox config profile create "+flagProfile+"` first, or drop --profile to target the default.",
							)
						}
						profileName = flagProfile
					case envProfile != "":
						if _, ok := cfg.Profiles[envProfile]; !ok {
							return output.NewCLIError(
								output.ErrUsage,
								"Unknown profile (URLBOX_PROFILE): "+envProfile,
								"Configured profiles: "+quotedSortedProfileNames(cfg.Profiles)+". Unset URLBOX_PROFILE or create the profile first.",
							)
						}
						profileName = envProfile
					default:
						profileName = cfg.DefaultProfile
						if profileName == "" {
							profileName = "default"
							cfg.DefaultProfile = profileName
						}
					}
					p := cfg.Profiles[profileName]

					// Overwrite guard (Round 1 S-C3): same-secret re-save is
					// idempotent; different-secret overwrite needs allowOverwrite.
					if p.APISecret != "" && p.APISecret != secret && !allowOverwrite {
						return output.NewCLIError(
							output.ErrConflict,
							fmt.Sprintf("profile %q already has an API secret (%s); overwrite refused", profileName, maskSecret(p.APISecret)),
							"Pass --force to overwrite, or use `urlbox config profile create <name>` for a separate profile. This guard prevents the 2026-05-08 incident class where an agent silently clobbers a real secret.",
						)
					}
					p.APISecret = secret
					cfg.Profiles[profileName] = p
					return nil
				})
			}

			err := runUpdate(force)
			if err != nil {
				// Interactive TTY: if the inner conflict was an overwrite
				// guard and we're on a TTY, prompt the user. On confirm,
				// retry the Update with allowOverwrite=true.
				var cli *output.CLIError
				if errors.As(err, &cli) && cli.Code == output.ErrConflict &&
					isStdinTTY(cmd.InOrStdin()) && isStderrTTY(cmd.ErrOrStderr()) {
					// Re-read just for the prompt display (out-of-lock; we
					// re-check inside Update on retry).
					if existing, _ := config.Load(); existing != nil {
						p := existing.Profiles[profileName]
						if !confirmAuthOverwrite(cmd, p.APISecret, secret) {
							return output.NewCLIError(
								output.ErrConflict,
								"auth cancelled — existing secret preserved",
								"Re-run with --force to overwrite without prompt, or use `urlbox config profile create <name>` for a separate profile.",
							)
						}
					}
					err = runUpdate(true)
				}
			}
			if err != nil {
				var cli *output.CLIError
				if errors.As(err, &cli) {
					return cli
				}
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
	c.Flags().StringVar(&apiSecret, "api-secret", "", "Urlbox API secret (skip the interactive prompt — leaks into ps and shell history; prefer --api-secret-stdin or --api-secret-file)")
	c.Flags().BoolVar(&apiSecretStdin, "api-secret-stdin", false, "Read the API secret from stdin until EOF (recommended for CI / agents)")
	c.Flags().StringVar(&apiSecretFile, "api-secret-file", "", "Read the API secret from the given file (trailing newline trimmed)")
	c.Flags().BoolVar(&force, "force", false, "Overwrite an existing secret on the default profile without confirmation (CI-safe escape hatch for the overwrite guard)")
	return c
}

// confirmAuthOverwrite prompts the user on stderr whether to replace the
// existing default-profile secret. Returns true if the user types y / yes
// (case-insensitive). Used only on interactive TTYs; non-TTY callers
// require --force instead.
func confirmAuthOverwrite(cmd *cobra.Command, existing, replacement string) bool {
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"Replacing existing secret %s with %s. Proceed? [y/N]: ",
		maskSecret(existing), maskSecret(replacement))
	answer, _ := authConfirmReader()
	_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

// maskSecret returns a redacted form of the API secret for safe display.
func maskSecret(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-2:]
}
