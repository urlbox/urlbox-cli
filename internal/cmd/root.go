package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
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
		cliErr = output.NewCLIError(output.ErrUsage, err.Error(), "")
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
	cmd.AddCommand(newRenderCmd())
	cmd.AddCommand(newSchemaCmd())
	cmd.AddCommand(newSkillCmd())

	return cmd
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
