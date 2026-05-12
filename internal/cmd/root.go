package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/validation"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// stderrTTYOverride forces the TTY result used by the root banner during tests.
// Nil = real detection.
var stderrTTYOverride *bool

// SetStderrTTYForTest forces IsStderrTTY to return v (test helper).
func SetStderrTTYForTest(v bool) { stderrTTYOverride = &v }

// ResetStderrTTYForTest clears the override (test helper).
func ResetStderrTTYForTest() { stderrTTYOverride = nil }

func isStderrTTY(w io.Writer) bool {
	if stderrTTYOverride != nil {
		return *stderrTTYOverride
	}
	return output.IsTTY(w)
}

const discoverabilityBanner = "If you're looking for the full list of commands in one place, try `urlbox commands` (or `urlbox commands --output-format json`)."

// Execute runs the root command with the given args, writing to the provided writers.
func Execute(args []string, stdout, stderr io.Writer) int {
	rootCmd := newRootCmd(stdout, stderr)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)

	err := rootCmd.Execute()
	if err == nil {
		return 0
	}

	// Resolve format for error output
	formatFlag, _ := rootCmd.PersistentFlags().GetString("output-format")
	format := output.ResolveFormat(formatFlag, stdout)
	styles := output.NewStylesForWriter(stdout)
	formatter := output.NewFormatter(format, styles)

	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) {
		msg := err.Error()
		hint := "Run `urlbox <command> --help` for usage, or `urlbox commands` for the full surface."
		// cobra.NoArgs produces "unknown command \"X\" for \"urlbox\"" with no
		// suggestion text (the suggestion-bearing legacyArgs path is bypassed
		// because we set Args=cobra.NoArgs to keep flag parsing happening
		// before arg validation). Compute the suggestion ourselves so the
		// did_you_mean behaviour is preserved.
		if suggestion, ok := suggestUnknownCommand(rootCmd, msg); ok {
			hint = `Did you mean "` + suggestion + `"? ` + hint
		} else if suggestion, ok := suggestUnknownFlag(rootCmd, args, msg); ok {
			// pflag's "unknown flag: --xxx" carries no suggestion either.
			hint = `Did you mean "--` + suggestion + `"? ` + hint
		}
		cliErr = output.NewCLIError(output.ErrUsage, msg, hint)
	}

	if !cliErr.Silent {
		env := output.NewErrorEnvelope(calledCommandFromArgs(rootCmd, args), cliErr)
		_ = formatter.WriteError(stdout, env)
	}
	return cliErr.ExitCode()
}

// calledCommandFromArgs returns the full invoked subcommand path
// (e.g. "config get", "config profile delete") by re-walking the args
// through the command tree. Cobra's CalledAs() reliably indicates the
// LEAF that was reached, but ancestor CalledAs values are not always
// retained on every code path, so deriving the path from args via
// Find() is the dependable source.
//
// Round 8 II: when no subcommand matched (bare `urlbox`, unknown
// command, unknown flag at root), returns the root name rather than
// "". The contract is that every error envelope carries a command —
// "urlbox" is more useful for agents than "".
func calledCommandFromArgs(root *cobra.Command, args []string) string {
	if rootCalled := root.CalledAs(); rootCalled != "" && rootCalled != root.Name() {
		return rootCalled
	}
	leaf, _, err := root.Find(args)
	if err != nil || leaf == nil || leaf == root {
		// Root command (or unresolved) — return root name so the
		// envelope always carries something.
		return root.Name()
	}
	// Walk up from the leaf to root, collecting names. CommandPath() returns
	// the binary-prefixed path like "urlbox config get"; strip the root name.
	path := leaf.CommandPath()
	prefix := root.Name() + " "
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, prefix)
	}
	return path
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
	var outputFormat string

	cmd := &cobra.Command{
		Use:     "urlbox",
		Short:   "Urlbox CLI — screenshots, PDFs, and more from the command line",
		Long:    "The official CLI for the Urlbox API. Render screenshots, PDFs, videos, and extracted content from URLs or HTML.",
		Version: version.Version,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(c *cobra.Command, args []string) error {
			// Reject unknown --output-format values up-front. Without this,
			// `--output-format ndjson` (and any other unknown value) silently
			// falls through to JSON in NewFormatter's default arm — confusing
			// for users who expect a different format. ndjson is reserved for
			// v1.1+ batch streaming.
			format, _ := c.Root().PersistentFlags().GetString("output-format")
			switch format {
			case "", "json", "text", "quiet":
				// OK
			case "ndjson":
				return output.NewCLIError(
					output.ErrUsage,
					`--output-format "ndjson" is not yet implemented`,
					"Coming in a future release alongside `urlbox batch`. Use json, text, or quiet today.",
				)
			default:
				return output.NewCLIError(
					output.ErrUsage,
					`unknown --output-format "`+format+`"`,
					"Use one of: json, text, quiet.",
				)
			}
			// Round 8 NN: --profile flag value validation. The Adv-4
			// `validateProfileName` rule applied to creation but not to
			// flag-resolution-time. So `--profile ""`, `--profile " "`,
			// `--profile $'\t'` silently behaved like "no flag" (the
			// resolver's `if flagProfile != ""` branch skipped them) —
			// confusing for agents that programmatically set the flag
			// from a variable that's sometimes empty. Now an explicit
			// blank rejects loudly.
			//
			// Uses Changed() rather than GetString() != "" so that
			// `--profile ""` (explicitly empty) is distinguished from
			// "flag not passed" (the latter is legitimately empty).
			if c.Root().PersistentFlags().Changed("profile") {
				profile, _ := c.Root().PersistentFlags().GetString("profile")
				if strings.TrimSpace(profile) == "" {
					return output.NewCLIError(
						output.ErrUsage,
						"--profile cannot be empty or whitespace-only",
						"Either omit --profile (uses the default profile / URLBOX_PROFILE / single profile), or pass an actual name.",
					)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if isStderrTTY(cmd.ErrOrStderr()) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), discoverabilityBanner)
			}
			return cmd.Help()
		},
	}

	cmd.SetVersionTemplate("urlbox " + version.Version + " (commit: " + version.Commit + ", built: " + version.Date + ")\n")
	cmd.PersistentFlags().StringVar(&outputFormat, "output-format", "", "Output format: json, text, or quiet")
	cmd.PersistentFlags().Bool("agent", false, "When combined with --help, output structured JSON help")
	cmd.PersistentFlags().String("jq", "", "Run a jq expression over the envelope (or .data with --output-format quiet)")
	cmd.PersistentFlags().String("profile", "", "Named config profile to use (overrides URLBOX_PROFILE)")

	defaultHelp := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		agent, _ := c.Flags().GetBool("agent")
		if !agent {
			defaultHelp(c, args)
			return
		}
		env := output.BuildAgentHelp(c)
		styles := output.NewStylesForWriter(c.OutOrStdout())
		formatter := output.NewFormatter(output.FormatJSON, styles)
		_ = formatter.WriteSuccess(c.OutOrStdout(), env)
	})

	cmd.AddCommand(newUpgradeCmd(stdout, stderr))
	cmd.AddCommand(newCommandsCmd(stdout, stderr))
	cmd.AddCommand(newSurfaceCmd(cmd))
	cmd.AddCommand(newAuthCmd())
	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newDashboardCmd())
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newLinkCmd())
	cmd.AddCommand(newPdfCmd())
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newSchemaCmd())
	cmd.AddCommand(newScreenshotCmd())
	cmd.AddCommand(newSkillCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newVideoCmd())

	return cmd
}

// suggestUnknownCommand inspects an error message of the form
// `unknown command "X" for "urlbox <parent-path>"` (emitted by
// cobra.NoArgs) and returns the closest known IMMEDIATE-subcommand name
// of that parent, if any.
//
// Round 6 BB class-fix: the previous version always walked root.Commands(),
// which meant `urlbox schema list` (where schema has no `list`) cross-
// suggested top-level commands like `link`. Now the parent path is
// parsed from the error string and the candidate pool is scoped to
// that parent's actual subcommands. The typed token is excluded as a
// belt-and-braces guard against the AA-class tautology emerging here.
func suggestUnknownCommand(root *cobra.Command, msg string) (string, bool) {
	// COBRA VERSION COUPLING: this prefix matches cobra v1.x emitted under
	// Args=cobra.NoArgs. A `go get -u cobra` reviewer should re-verify.
	const prefix = `unknown command "`
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return "", false
	}
	rest := msg[idx+len(prefix):]
	end := strings.IndexByte(rest, '"')
	if end <= 0 {
		return "", false
	}
	typed := rest[:end]

	// Parent path: cobra appends ` for "urlbox <parent...>"` after the
	// typed token. The ` for "` substring starts at the same `"` that
	// closes the typed name, so we look for it at-or-after `end`.
	parent := root
	if forIdx := strings.Index(rest, `" for "`); forIdx >= end {
		parentPath := rest[forIdx+len(`" for "`):]
		if closeQuote := strings.IndexByte(parentPath, '"'); closeQuote > 0 {
			parentPath = parentPath[:closeQuote]
		}
		// Strip the root's own name (e.g. "urlbox foo bar" -> ["foo","bar"])
		// and walk the tree.
		tokens := strings.Fields(parentPath)
		if len(tokens) > 0 && tokens[0] == root.Name() {
			tokens = tokens[1:]
		}
		if c, _, err := root.Find(tokens); err == nil && c != nil {
			parent = c
		}
	}

	candidates := make([]string, 0, len(parent.Commands()))
	for _, sub := range parent.Commands() {
		if !sub.IsAvailableCommand() {
			continue
		}
		if sub.Name() == typed {
			continue // belt-and-braces (matches AA's class-fix shape)
		}
		candidates = append(candidates, sub.Name())
	}
	return validation.ClosestMatch(typed, candidates)
}

// suggestUnknownFlag inspects an error message of the form
// `unknown flag: --xxx` (emitted by pflag) and returns the closest known
// long flag name on the ACTIVE subcommand (plus its inherited persistent
// flags). The flag prefix (`--`) is NOT included in the returned name.
//
// Round 6 AA class-fix: the candidate pool used to be the union of every
// flag in the command tree. A typo on one command could match a flag
// from an unrelated command — including the rejected flag itself when
// it happened to live on a sibling. `urlbox render --url` would suggest
// "--url" back (render takes positional URL; --url lives on link). The
// pool is now scoped to the active command, and the rejected token is
// explicitly excluded as a belt-and-braces guard against the tautology
// re-emerging via some future fuzzy-match edge case.
func suggestUnknownFlag(root *cobra.Command, args []string, msg string) (string, bool) {
	// PFLAG VERSION COUPLING: this prefix matches pflag's "unknown flag:"
	// error string. A `go get -u pflag` reviewer should re-verify.
	const prefix = "unknown flag: --"
	idx := strings.Index(msg, prefix)
	if idx < 0 {
		return "", false
	}
	rest := msg[idx+len(prefix):]
	// Stop at first whitespace or newline (pflag emits a single line).
	if cut := strings.IndexAny(rest, " \t\n"); cut >= 0 {
		rest = rest[:cut]
	}
	// Trim any trailing punctuation.
	typed := strings.TrimRight(rest, ".,;:")
	if typed == "" {
		return "", false
	}

	// Scope the candidate pool to the active subcommand. cobra.Find walks
	// args and returns the deepest matched command; flags on its parents
	// reach it via inheritance, so we collect them too.
	active := findActiveCommand(root, args)
	candidates := collectActiveFlagNames(active, typed)
	return validation.ClosestMatch(typed, candidates)
}

// findActiveCommand returns the deepest cobra.Command matched by args.
// Falls back to root when no subcommand matches (e.g. typo at the root).
func findActiveCommand(root *cobra.Command, args []string) *cobra.Command {
	c, _, err := root.Find(args)
	if err != nil || c == nil {
		return root
	}
	return c
}

// collectActiveFlagNames returns the long-flag-name pool for the active
// command: its own flags + every ancestor's persistent flags. The exact
// rejected token (`typed`) is explicitly excluded — even if the same
// name happens to live on the command, suggesting it back is never the
// right answer.
func collectActiveFlagNames(active *cobra.Command, typed string) []string {
	seen := map[string]struct{}{}
	for c := active; c != nil; c = c.Parent() {
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name != typed {
				seen[f.Name] = struct{}{}
			}
		})
	}
	active.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name != typed {
			seen[f.Name] = struct{}{}
		}
	})
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}
