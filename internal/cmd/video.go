package cmd

import "github.com/spf13/cobra"

// newVideoCmd is a thin alias over `urlbox render --format mp4`. Video
// renders are typically slower; users may also want --async + --webhook-url
// for production workflows, but the alias pre-sets only the format default.
func newVideoCmd() *cobra.Command {
	f := &renderFlags{}
	c := &cobra.Command{
		Use:   "video [url]",
		Short: "Render a URL as MP4 video (alias for `render --format mp4`)",
		Args:  cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("format") {
				return cmd.Flags().Set("format", "mp4")
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
