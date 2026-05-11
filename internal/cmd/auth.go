package cmd

import (
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
			secret, cliErr := resolveAPISecretInput(secretStdin, cmd.ErrOrStderr(), apiSecret, apiSecretStdin, apiSecretFile)
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

			// Overwrite guard (Round 1 S-C3): protect against the 2026-05-08
			// incident class where an autonomous agent runs `urlbox auth
			// --api-secret <test_value>` and silently clobbers the user's
			// real secret. Only fires when the new value differs from the
			// existing — same-secret re-save remains idempotent.
			// Both branches return ErrConflict — the existing secret is the
			// conflict in both cases, and unified exit code 7 lets CI scripts
			// classify "we did not save" without caring whether the user typed
			// 'n' or whether they forgot --force. The error message and code
			// field still distinguish the variants for humans / structured
			// log consumers.
			if p.APISecret != "" && p.APISecret != secret && !force {
				if isStdinTTY(cmd.InOrStdin()) && isStderrTTY(cmd.ErrOrStderr()) {
					if !confirmAuthOverwrite(cmd, p.APISecret, secret) {
						return output.NewCLIError(
							output.ErrConflict,
							"auth cancelled — existing secret preserved",
							"Re-run with --force to overwrite without prompt, or use `urlbox config profile create <name>` for a separate profile.",
						)
					}
				} else {
					return output.NewCLIError(
						output.ErrConflict,
						fmt.Sprintf("profile %q already has an API secret (%s); overwrite refused", profileName, maskSecret(p.APISecret)),
						"Pass --force to overwrite, or use `urlbox config profile create <name>` for a separate profile. This guard prevents the 2026-05-08 incident class where an agent silently clobbers a real secret.",
					)
				}
			}

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
