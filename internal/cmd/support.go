package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/browser"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// supportURL is the Urlbox contact page — the canonical support entry
// point; /support has no dedicated page.
const supportURL = "https://urlbox.com/contact"

// supportOpener is the browser opener used by the support command. Own var
// so test injection doesn't cross-contaminate dashboard's.
var supportOpener browser.Opener = browser.NewOSOpener()

// SetSupportOpenerForTest swaps in a fake browser.Opener. Pair with
// t.Cleanup(ResetSupportOpenerForTest).
func SetSupportOpenerForTest(o browser.Opener) { supportOpener = o }

// ResetSupportOpenerForTest restores the production OSOpener.
func ResetSupportOpenerForTest() { supportOpener = browser.NewOSOpener() }

func newSupportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "support",
		Short: "Open the Urlbox support contact page in your browser",
		Long: `Opens https://urlbox.com/contact in your default browser.

On headless environments the URL is printed to stderr instead. The
standard envelope is still emitted on stdout so agents and pipelines can
rely on the same shape regardless of host.

Exit codes:
  0   browser launched, or URL printed in headless mode
  10  the OS browser handler returned an error (URL is in the hint)`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runSupport(c)
		},
	}
}

func runSupport(c *cobra.Command) error {
	data := map[string]any{"url": supportURL}
	formatFlag, _ := c.Root().PersistentFlags().GetString("output-format")
	resolvedFormat := output.ResolveFormat(formatFlag, c.OutOrStdout())
	if resolvedFormat == output.FormatJSON || resolvedFormat == output.FormatQuiet {
		return writeSupportEnvelope(c, data,
			"Support URL emitted (no browser launched in machine-readable mode)",
			[]output.Breadcrumb{{Action: "copy", Cmd: supportURL}})
	}
	if isHeadless() {
		_, _ = fmt.Fprintln(c.ErrOrStderr(),
			"Support URL: "+supportURL+"  (open in any browser)")
		return writeSupportEnvelope(c, data,
			"Support URL printed (no graphical session detected)",
			[]output.Breadcrumb{{Action: "copy", Cmd: supportURL}})
	}
	if err := supportOpener.Open(supportURL); err != nil {
		return output.NewCLIError(
			output.ErrServer,
			"Failed to open browser: "+err.Error(),
			"Open this URL manually: "+supportURL,
		)
	}
	return writeSupportEnvelope(c, data,
		"Opened "+supportURL,
		[]output.Breadcrumb{{Action: "browse", Cmd: supportURL}})
}

// writeSupportEnvelope mirrors writeDashboardEnvelope: quiet emits the
// bare URL for pipelines, everything else the standard envelope.
func writeSupportEnvelope(c *cobra.Command, data map[string]any, summary string, breadcrumbs []output.Breadcrumb) error {
	formatFlag, _ := c.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := c.Root().PersistentFlags().GetString("jq")
	stdout := c.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if format == output.FormatQuiet && jqExpr == "" {
		_, err := fmt.Fprintln(stdout, supportURL)
		return err
	}
	env := output.NewEnvelope("support", data, summary, breadcrumbs)
	return writeEnvelope(c, env)
}
