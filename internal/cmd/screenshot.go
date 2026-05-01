package cmd

import "github.com/spf13/cobra"

// newScreenshotCmd is a thin alias over `urlbox render --format png`. It
// shares the entire render pipeline via attachRenderFlags + runRender —
// only the default --format value is pre-set. Users who pass --format
// explicitly override the alias default.
func newScreenshotCmd() *cobra.Command {
	f := &renderFlags{}
	c := &cobra.Command{
		Use:     "screenshot [url]",
		Short:   "Capture a screenshot (alias for `render --format png`)",
		Aliases: []string{"shot"},
		Args:    cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("format") {
				return cmd.Flags().Set("format", "png")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRender(cmd, args, f)
		},
	}
	attachRenderFlags(c, f)
	return c
}
