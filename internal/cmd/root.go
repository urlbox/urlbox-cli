package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/urlbox/cli/internal/version"
)

// Execute runs the root command with the given args, writing to the provided writers.
func Execute(args []string, stdout, stderr io.Writer) int {
	rootCmd := newRootCmd(stdout, stderr)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)

	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func newRootCmd(stdout, stderr io.Writer) *cobra.Command {
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
			return cmd.Help()
		},
	}

	cmd.SetVersionTemplate(fmt.Sprintf("urlbox %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date))

	return cmd
}
