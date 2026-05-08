// internal/cmd/dashboard.go
package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/browser"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// dashboardURL is the canonical SaaS dashboard. Self-hosted deployments
// are not currently configurable here — when raised by users we'll plumb
// through the profile config the same way --api-host is handled.
const dashboardURL = "https://urlbox.com/dashboard"

// dashboardOpener is the browser opener used by the dashboard command.
// Production builds get OSOpener; tests swap a fake via
// SetDashboardOpenerForTest. Distinct from render's renderOpener so the
// two commands' test injections don't cross-contaminate.
var dashboardOpener browser.Opener = browser.NewOSOpener()

// isHeadless detects whether the host can plausibly launch a browser.
// Wrapped in a var so tests pin the result (CI runs headless on Linux,
// dev runs non-headless on macOS — both code paths must be reachable
// from a single test binary).
var isHeadless = detectHeadless

// SetDashboardOpenerForTest swaps in a fake browser.Opener for the
// dashboard command. Pair with t.Cleanup(ResetDashboardOpenerForTest).
func SetDashboardOpenerForTest(o browser.Opener) { dashboardOpener = o }

// ResetDashboardOpenerForTest restores production OSOpener.
func ResetDashboardOpenerForTest() { dashboardOpener = browser.NewOSOpener() }

// SetHeadlessDetectorForTest swaps the headless detector. Pair with
// t.Cleanup(ResetHeadlessDetectorForTest).
func SetHeadlessDetectorForTest(fn func() bool) { isHeadless = fn }

// ResetHeadlessDetectorForTest restores the real detector.
func ResetHeadlessDetectorForTest() { isHeadless = detectHeadless }

// detectHeadless returns true when no graphical session is reachable.
//
//	linux           → headless iff DISPLAY and WAYLAND_DISPLAY are both empty
//	darwin, windows → never headless (the OS handler always works)
//	other (BSD…)    → assumed headless; xdg-open may or may not be installed
//
// Conservative bias: when in doubt, fall back to printing the URL rather
// than firing a process that may hang or fail silently.
func detectHeadless() bool {
	switch runtime.GOOS {
	case "linux":
		return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
	case "darwin", "windows":
		return false
	default:
		return true
	}
}

func newDashboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: "Open the Urlbox dashboard in your browser",
		Long: `Opens https://urlbox.com/dashboard in your default browser.

On headless environments (no DISPLAY / WAYLAND_DISPLAY on Linux, or an
unsupported OS) the URL is printed to stderr instead. The standard
envelope is still emitted on stdout so agents and pipelines can rely on
the same shape regardless of host.

Exit codes:
  0   browser launched, or URL printed in headless mode
  10  the OS browser handler returned an error (URL is in the hint)`,
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runDashboard(c)
		},
	}
}

func runDashboard(c *cobra.Command) error {
	data := map[string]any{"url": dashboardURL}

	if isHeadless() {
		// Stderr is for humans; stdout still carries the envelope.
		_, _ = fmt.Fprintln(c.ErrOrStderr(),
			"Dashboard URL: "+dashboardURL+"  (open in any browser)")
		return writeDashboardEnvelope(c, data,
			"Dashboard URL printed (no graphical session detected)",
			[]output.Breadcrumb{{Action: "copy", Cmd: dashboardURL}})
	}

	if err := dashboardOpener.Open(dashboardURL); err != nil {
		return output.NewCLIError(
			output.ErrServer,
			"Failed to open browser: "+err.Error(),
			"Open this URL manually: "+dashboardURL,
		)
	}
	return writeDashboardEnvelope(c, data,
		"Opened "+dashboardURL,
		[]output.Breadcrumb{{Action: "browse", Cmd: dashboardURL}})
}

// writeDashboardEnvelope emits the standard envelope. Quiet mode is
// bespoke: the bare URL on stdout (no JSON quoting), matching link's
// quiet contract so `urlbox dashboard --output-format quiet | xargs ...`
// pipelines work the way an agent expects.
func writeDashboardEnvelope(c *cobra.Command, data map[string]any, summary string, breadcrumbs []output.Breadcrumb) error {
	formatFlag, _ := c.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := c.Root().PersistentFlags().GetString("jq")
	stdout := c.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if format == output.FormatQuiet && jqExpr == "" {
		_, err := fmt.Fprintln(stdout, dashboardURL)
		return err
	}
	env := output.NewEnvelope("dashboard", data, summary, breadcrumbs)
	return writeEnvelope(c, env)
}
