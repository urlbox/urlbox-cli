package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// reportHintLine is the post-render affordance: one ready-to-run command
// carrying the render id.
func reportHintLine(renderID string) string {
	return "Something wrong with this render? Report it: urlbox report " + renderID
}

// renderIDForHint picks the id the hint should name: async bodies carry
// renderId; sync bodies carry nothing, so the header-captured Response
// field is the fallback.
func renderIDForHint(resp *api.Response) string {
	if id, ok := resp.Data["renderId"].(string); ok && id != "" {
		return id
	}
	return resp.RenderID
}

// hintGatesPass centralises the affordance rulings: text output only,
// stderr TTY only. json/quiet stay byte-identical for agents.
func hintGatesPass(cmd *cobra.Command) bool {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	if output.ResolveFormat(formatFlag, cmd.OutOrStdout()) != output.FormatText {
		return false
	}
	return isStderrTTY(cmd.ErrOrStderr())
}

// printReportHint writes the muted hint line to stderr after a render
// result. No-op when any gate fails or no id was captured.
func printReportHint(cmd *cobra.Command, renderID string) {
	if renderID == "" || !hintGatesPass(cmd) {
		return
	}
	styles := output.NewStylesForWriter(cmd.ErrOrStderr())
	_, _ = fmt.Fprintln(cmd.ErrOrStderr(), styles.Muted.Render("  "+reportHintLine(renderID)))
}

// appendReportHint rides the failed render's existing Hint (printed under
// the Error: line in text mode) so the report affordance lands below the
// error rather than above it. The mutation only happens when the text+TTY
// gates pass, so json/quiet error envelopes never carry it.
func appendReportHint(cmd *cobra.Command, err error) error {
	var ridErr *api.RenderIDError
	if !errors.As(err, &ridErr) || ridErr.RenderID == "" || !hintGatesPass(cmd) {
		return err
	}
	if ridErr.Err.Hint != "" {
		ridErr.Err.Hint += "\n"
	}
	ridErr.Err.Hint += reportHintLine(ridErr.RenderID)
	return err
}
