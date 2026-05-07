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
		} else if suggestion, ok := suggestUnknownFlag(rootCmd, msg); ok {
			// pflag's "unknown flag: --xxx" carries no suggestion either.
			hint = `Did you mean "--` + suggestion + `"? ` + hint
		}
		cliErr = output.NewCLIError(output.ErrUsage, msg, hint)
	}

	if !cliErr.Silent {
		env := output.NewErrorEnvelope(calledCommand(rootCmd), cliErr)
		_ = formatter.WriteError(stdout, env)
	}
	return cliErr.ExitCode()
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
	cmd.AddCommand(newDoctorCmd())
	cmd.AddCommand(newPdfCmd())
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newSchemaCmd())
	cmd.AddCommand(newScreenshotCmd())
	cmd.AddCommand(newSkillCmd())
	cmd.AddCommand(newVideoCmd())

	return cmd
}

// suggestUnknownCommand inspects an error message of the form
// `unknown command "X" for "..."` (emitted by cobra.NoArgs) and returns the
// closest known immediate-subcommand name of root, if any.
//
// Known limitation (tracked for v0.12.0+ polish): only walks root.Commands().
// Subcommand-level typos (e.g. `urlbox config gett`) won't be suggested
// because the parent path is not parsed out of the error message.
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
	candidates := make([]string, 0, len(root.Commands()))
	for _, sub := range root.Commands() {
		if sub.IsAvailableCommand() {
			candidates = append(candidates, sub.Name())
		}
	}
	return validation.ClosestMatch(typed, candidates)
}

// suggestUnknownFlag inspects an error message of the form
// `unknown flag: --xxx` (emitted by pflag) and returns the closest known
// long flag name across the root and all its subcommands. The flag prefix
// (`--`) is NOT included in the returned name.
//
// Known limitation (tracked for v0.12.0+ polish): the candidate pool is the
// UNION of every flag in the command tree. A typo on one command can match
// a flag from an unrelated command (e.g. `urlbox auth status --widht`
// would suggest `--width`, a render flag). Scoping per active command would
// require parsing the active command path out of pflag's error string,
// which doesn't include it. Defensible as-is for v0.12.0; revisit later.
func suggestUnknownFlag(root *cobra.Command, msg string) (string, bool) {
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

	candidates := collectFlagNames(root)
	return validation.ClosestMatch(typed, candidates)
}

// collectFlagNames returns the union of long flag names defined on cmd and
// all its descendants (deduplicated).
func collectFlagNames(cmd *cobra.Command) []string {
	seen := map[string]struct{}{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) { seen[f.Name] = struct{}{} })
		c.PersistentFlags().VisitAll(func(f *pflag.Flag) { seen[f.Name] = struct{}{} })
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cmd)
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}

// calledCommand returns the name of the subcommand that was invoked, or empty string.
func calledCommand(cmd *cobra.Command) string {
	if cmd.CalledAs() != "" && cmd.CalledAs() != cmd.Name() {
		return cmd.CalledAs()
	}
	for _, c := range cmd.Commands() {
		if c.CalledAs() != "" {
			return c.CalledAs()
		}
	}
	return ""
}
