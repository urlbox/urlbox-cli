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
		Long: `Render a URL as MP4 video. Thin alias over ` + "`render --format mp4`" + `.

The Urlbox API uses a single ` + "`/v1/screenshot`" + ` endpoint for every render
format — pdf, mp4, png, webp, etc. all POST to the same path, with the
output format specified in the JSON body. So ` + "`urlbox video --curl`" + ` will
show ` + "`/v1/screenshot`" + ` in the URL — that's correct, not a bug.

Video renders are typically slower than screenshots; consider ` + "`--async`" + `
with ` + "`--webhook-url`" + ` for production workflows.`,
		Args: cobra.MaximumNArgs(1),
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
