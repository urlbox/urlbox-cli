package cmd

import "github.com/spf13/cobra"

// newPdfCmd is a thin alias over `urlbox render --format pdf --full-page`.
// Pre-sets both defaults; either can be overridden by the user.
func newPdfCmd() *cobra.Command {
	f := &renderFlags{}
	c := &cobra.Command{
		Use:   "pdf [url]",
		Short: "Render a URL as PDF (alias for `render --format pdf --full-page`)",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("format") {
				if err := cmd.Flags().Set("format", "pdf"); err != nil {
					return err
				}
			}
			if !cmd.Flags().Changed("full-page") {
				return cmd.Flags().Set("full-page", "true")
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
