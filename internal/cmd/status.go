// internal/cmd/status.go
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// statusClientOverride is a test injection point. Production builds always
// see nil and construct a real api.HTTPClient.
var statusClientOverride api.Client

// SetStatusClientForTest swaps in a fake api.Client for the status command.
// Pair with t.Cleanup(ResetStatusClientForTest).
func SetStatusClientForTest(c api.Client) { statusClientOverride = c }

// ResetStatusClientForTest restores production client construction.
func ResetStatusClientForTest() { statusClientOverride = nil }

// defaultStatusTimeout is the per-call deadline for the status GET. Status
// is cheap; users who want long polling reach for --wait + --timeout (Task 4).
const defaultStatusTimeout = 30 * time.Second

// defaultStatusPollInterval is the time between successive GETs when --wait
// is in use (Task 4 wires up the polling loop; Task 3 only registers the flag).
const defaultStatusPollInterval = 2 * time.Second

// statusFlags carries every convenience flag the status command supports.
type statusFlags struct {
	apiSecret    string
	timeout      time.Duration
	wait         bool
	pollInterval time.Duration
	noRetry      bool
	maxRetries   int
}

func newStatusCmd() *cobra.Command {
	f := &statusFlags{}
	c := &cobra.Command{
		Use:   "status <renderId>",
		Short: "Look up the state of an async render",
		Long: `Look up the state of an async render by renderId.

Without --wait, returns immediately with the current status. With --wait,
polls until the render reaches a terminal state (succeeded / failed) or the
--timeout elapses (Task 4 — not yet implemented).

Exit codes:
  0   succeeded, or in-flight (created / retrying)
  5   renderId not found
  10  the API reported status=failed (the render itself failed)
  11  network failure

Examples:
  urlbox status ps_abc123
  urlbox status ps_abc123 --output-format json
  urlbox status ps_abc123 --wait                  # blocks until terminal (Task 4)`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, f)
		},
	}
	c.Flags().StringVar(&f.apiSecret, "api-secret", "",
		"Per-call override of the API secret (else read from config / env)")
	c.Flags().DurationVar(&f.timeout, "timeout", defaultStatusTimeout,
		"Per-call timeout for the status GET (e.g. 30s, 2m). With --wait, "+
			"caps the total time spent polling.")
	c.Flags().BoolVar(&f.wait, "wait", false,
		"Poll until the render reaches a terminal state (Task 4 — not yet implemented)")
	c.Flags().DurationVar(&f.pollInterval, "poll-interval", defaultStatusPollInterval,
		"Time between successive status GETs when --wait is set")
	c.Flags().BoolVar(&f.noRetry, "no-retry", false, "Disable automatic retries on 429 / 5xx")
	c.Flags().IntVar(&f.maxRetries, "max-retries", api.DefaultRetryConfig().MaxRetries,
		"Maximum retry attempts on 429 / 5xx")
	return c
}

func runStatus(cmd *cobra.Command, args []string, f *statusFlags) error {
	if len(args) == 0 {
		return output.NewCLIError(
			output.ErrUsage,
			"missing required renderId",
			"Provide the renderId as the first positional arg: `urlbox status <renderId>`. "+
				"You can get one from `urlbox render <url> --async`.",
		)
	}
	renderID := args[0]

	if f.wait {
		// Task 4 implements the polling loop. Until then, surface a clear
		// usage error so callers passing --wait don't get silently ignored.
		return output.NewCLIError(
			output.ErrUsage,
			"--wait not yet implemented",
			"Re-run without --wait, or upgrade to a CLI version where Task 4 has shipped.",
		)
	}

	client, cerr := buildStatusClient(cmd, f)
	if cerr != nil {
		return cerr
	}

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	resp, err := client.Status(ctx, renderID)
	if err != nil {
		// HTTPClient already returns *output.CLIError (404 → not_found,
		// network → network, etc) via mapStatusToCLIError + the timeout
		// branch in do(). Propagate as-is.
		var cli *output.CLIError
		if errors.As(err, &cli) {
			return cli
		}
		return output.NewCLIError(output.ErrServer, err.Error(), "Run `urlbox doctor` to verify connectivity.")
	}

	return writeStatusEnvelope(cmd, resp, renderID)
}

// writeStatusEnvelope inspects the API's status field and emits the right
// envelope (success or error). Status enum from the spec:
//
//	created    → in-flight (success envelope, breadcrumb to --wait)
//	retrying   → in-flight (same)
//	processing → in-flight (alias used by some response paths)
//	succeeded  → terminal-OK (success envelope, breadcrumb to open URL)
//	failed     → terminal-error → ErrServer (exit 10)
//	error      → terminal-error → ErrServer (alias for failed)
//
// not-found is handled at the HTTP layer via 404 → ErrNotFound — it never
// reaches this switch.
func writeStatusEnvelope(cmd *cobra.Command, resp *api.Response, renderID string) error {
	statusStr, _ := resp.Data["status"].(string)
	switch statusStr {
	case "succeeded":
		renderURL, _ := resp.Data["renderUrl"].(string)
		summary := fmt.Sprintf("Render %s succeeded", renderID)
		if renderURL != "" {
			summary = fmt.Sprintf("Render %s succeeded — %s", renderID, renderURL)
		}
		breadcrumbs := []output.Breadcrumb{}
		if renderURL != "" {
			// curl -O is cross-platform (macOS, Linux, Windows) and reflects
			// what an agent typically wants — the bytes — rather than a
			// browser tab. macOS-only `open` would fail on other platforms.
			breadcrumbs = append(breadcrumbs, output.Breadcrumb{
				Action: "download",
				Cmd:    "curl -O " + renderURL,
			})
		}
		env := output.NewEnvelope("status", resp.Data, summary, breadcrumbs)
		return writeEnvelope(cmd, env)

	case "failed", "error":
		errMsg, _ := resp.Data["error"].(string)
		if errMsg == "" {
			errMsg = fmt.Sprintf("Render %s failed", renderID)
		} else {
			errMsg = fmt.Sprintf("Render %s failed: %s", renderID, errMsg)
		}
		return output.NewCLIError(
			output.ErrServer,
			errMsg,
			"Inspect the render in the dashboard, or re-run `urlbox render <url>` with adjusted options.",
		)

	default:
		// In-flight: created, retrying, processing, or any future enum
		// value the API might add. Default to ok=true so future enums
		// don't auto-bomb the agent — Task 4's --wait will gate on terminal
		// states explicitly. If the API ever introduces a new terminal
		// status (cancelled, expired, etc.), warn on stderr so an operator
		// notices the misclassification even though we keep the agent path
		// non-fatal.
		if statusStr != "" && !isKnownInflight(statusStr) {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: unrecognized status %q; treating as in-flight\n", statusStr)
		}
		summary := fmt.Sprintf("Render %s is %s", renderID, fallbackStatusLabel(statusStr))
		bc := []output.Breadcrumb{
			{
				Action: "wait",
				Cmd:    fmt.Sprintf("urlbox status %s --wait", renderID),
			},
		}
		env := output.NewEnvelope("status", resp.Data, summary, bc)
		return writeEnvelope(cmd, env)
	}
}

// knownInflightStatuses is the set of non-terminal status strings the API
// currently returns. Used purely to detect unrecognized values so we can
// warn on stderr — the in-flight code path itself accepts anything.
var knownInflightStatuses = map[string]struct{}{
	"created":    {},
	"retrying":   {},
	"processing": {},
	"queued":     {},
}

// isKnownInflight reports whether s is a known non-terminal status. The
// caller has already established s is not "succeeded"/"failed"/"error" and
// is non-empty.
func isKnownInflight(s string) bool {
	_, ok := knownInflightStatuses[s]
	return ok
}

// fallbackStatusLabel returns a human-friendly word for the status value,
// avoiding "Render ps_x is " (empty) when the API returns no status field.
func fallbackStatusLabel(s string) string {
	if s == "" {
		return "in flight"
	}
	return s
}

// buildStatusClient returns the test-injected client if present, else a
// production HTTPClient resolved from env/config — same wiring as render.
func buildStatusClient(cmd *cobra.Command, f *statusFlags) (api.Client, *output.CLIError) {
	if statusClientOverride != nil {
		return statusClientOverride, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
	}
	profile, _ := cmd.Root().PersistentFlags().GetString("profile")
	resolved, rerr := config.Resolve(config.ResolveOptions{
		FlagAPISecret: f.apiSecret,
		FlagProfile:   profile,
		EnvAPISecret:  os.Getenv(config.EnvAPISecret),
		EnvAPIHost:    os.Getenv(config.EnvAPIHost),
		EnvProfile:    os.Getenv(config.EnvProfile),
		Config:        cfg,
	})
	if rerr != nil {
		var cli *output.CLIError
		if errors.As(rerr, &cli) {
			return nil, cli
		}
		return nil, output.NewCLIError(
			output.ErrUsage,
			rerr.Error(),
			"Check your config with `urlbox config show`, or set --api-secret / URLBOX_API_SECRET explicitly.",
		)
	}

	host := resolved.APIHost
	if host == "" {
		host = api.ResolveAPIHost()
	}

	c := api.NewHTTPClient(host, resolved.APIKey, resolved.APISecret)
	if f.noRetry {
		c.Retry = api.NoRetryConfig()
	} else {
		c.Retry.MaxRetries = f.maxRetries
	}
	return c, nil
}
