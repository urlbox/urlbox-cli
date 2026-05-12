// internal/cmd/status.go
package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// statusClock is the wall-clock source used by runStatusWait. Production code
// gets the real clock; tests swap a FakeClock via SetStatusClockForTest so the
// polling loop runs in microseconds.
var statusClock clock.Clock = clock.New()

// statusClientOverride is a test injection point. Production builds always
// see nil and construct a real api.HTTPClient.
var statusClientOverride api.Client

// SetStatusClientForTest swaps in a fake api.Client for the status command.
// Pair with t.Cleanup(ResetStatusClientForTest).
func SetStatusClientForTest(c api.Client) { statusClientOverride = c }

// ResetStatusClientForTest restores production client construction.
func ResetStatusClientForTest() { statusClientOverride = nil }

// SetStatusClockForTest swaps the package-level statusClock for tests.
// Mirrors SetStatusClientForTest's shape — no return value; pair with
// ResetStatusClockForTest in t.Cleanup.
func SetStatusClockForTest(c clock.Clock) { statusClock = c }

// ResetStatusClockForTest restores the real wall clock.
func ResetStatusClockForTest() { statusClock = clock.New() }

// defaultStatusTimeout is the per-call deadline for the status GET. Status
// is cheap; users who want long polling reach for --wait + --timeout (Task 4).
const defaultStatusTimeout = 60 * time.Second

// defaultStatusPollInterval is the time between successive GETs when --wait
// is in use (Task 4 wires up the polling loop; Task 3 only registers the flag).
const defaultStatusPollInterval = 2 * time.Second

// statusFlags carries every convenience flag the status command supports.
type statusFlags struct {
	apiSecret      string
	apiSecretStdin bool
	apiSecretFile  string
	timeout        time.Duration
	wait           bool
	pollInterval   time.Duration
	noRetry        bool
	maxRetries     int
}

func newStatusCmd() *cobra.Command {
	f := &statusFlags{}
	c := &cobra.Command{
		Use:   "status <renderId>",
		Short: "Look up the state of an async render",
		Long: `Look up the state of an async render by renderId.

Without --wait, returns immediately with the current status. With --wait,
polls until the render reaches a terminal state (succeeded / failed) or the
--timeout elapses.

Exit codes:
  0   succeeded, or in-flight (created / retrying) without --wait
  1   usage error (missing renderId, invalid flag)
  5   renderId not found
  10  the API reported status=failed (the render itself failed)
  11  network failure, or --wait timed out before reaching a terminal state

Examples:
  urlbox status ps_abc123
  urlbox status ps_abc123 --output-format json
  urlbox status ps_abc123 --wait
  urlbox status ps_abc123 --wait --timeout 2m --poll-interval 5s`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, args, f)
		},
	}
	c.Flags().StringVar(&f.apiSecret, "api-secret", "",
		"Per-call override of the API secret (leaks via ps + shell history; prefer --api-secret-stdin or --api-secret-file)")
	c.Flags().BoolVar(&f.apiSecretStdin, "api-secret-stdin", false, "Read the API secret from stdin until EOF")
	c.Flags().StringVar(&f.apiSecretFile, "api-secret-file", "", "Read the API secret from the given file (trailing newline trimmed)")
	c.Flags().DurationVar(&f.timeout, "timeout", defaultStatusTimeout,
		"Per-call timeout for the status GET (e.g. 30s, 2m). With --wait, "+
			"caps the total time spent polling.")
	c.Flags().BoolVar(&f.wait, "wait", false,
		"Poll until the render reaches a terminal state (succeeded / failed) or --timeout elapses")
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
	// Reject empty / whitespace-only renderIDs locally. Without this,
	// `urlbox status ""` would forward an empty path segment to the API,
	// 404, and surface "Not found" with exit 5 — masking what's actually a
	// user-input bug. TrimSpace also handles `urlbox status "   "`.
	renderID := strings.TrimSpace(args[0])
	if renderID == "" {
		return output.NewCLIError(
			output.ErrUsage,
			"missing render ID (got empty string)",
			"Provide the renderId as the first positional arg, e.g.: urlbox status ps_abc123. "+
				"You can get one from `urlbox render <url> --async`.",
		)
	}

	if resolved, cliErr := resolveAPISecretInput(secretStdin, cmd.ErrOrStderr(), f.apiSecret, cmd.Flags().Changed("api-secret"), f.apiSecretStdin, f.apiSecretFile); cliErr != nil {
		return cliErr
	} else if resolved != "" {
		f.apiSecret = resolved
	}

	client, cerr := buildStatusClient(cmd, f)
	if cerr != nil {
		return cerr
	}

	if f.wait {
		return runStatusWait(cmd, client, renderID, f)
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

// runStatusWait polls client.Status every f.pollInterval until the render
// reaches a terminal state (succeeded / failed / error), the deadline elapses,
// or the context is cancelled.
//
// Terminal handling reuses writeStatusEnvelope so the success / failure
// envelopes match the single-shot path exactly. Mid-poll non-terminal states
// (created, retrying, processing, queued, …) are silently absorbed — the
// breadcrumb-to---wait suggestion only matters when the user asked for a
// snapshot, not while we're already waiting.
func runStatusWait(cmd *cobra.Command, client api.Client, renderID string, f *statusFlags) error {
	if f.timeout <= 0 {
		return output.NewCLIError(
			output.ErrUsage,
			"--timeout must be positive when --wait is set",
			"Use a positive duration like --timeout 2m.",
		)
	}
	if f.pollInterval <= 0 {
		return output.NewCLIError(
			output.ErrUsage,
			"--poll-interval must be positive",
			"Use a positive duration like --poll-interval 2s.",
		)
	}

	start := statusClock.Now()
	deadline := start.Add(f.timeout)
	lastStatus := ""

	// Per-poll context = the remaining budget. Each poll is allowed to
	// consume up to (deadline - now), so a single hung GET can't burn past
	// the user's --timeout. Bounded by the API's own behaviour: HTTPClient
	// already retries 429/5xx within its own budget.
	attempt := 0
	for {
		now := statusClock.Now()
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			return waitTimeoutError(renderID, f.timeout, lastStatus)
		}

		ctx, cancel := context.WithTimeout(context.Background(), remaining)
		resp, err := client.Status(ctx, renderID)
		cancel()
		if err != nil {
			// Propagate *output.CLIError straight through (404 → not_found,
			// network, timeout). Same shape as single-shot.
			var cli *output.CLIError
			if errors.As(err, &cli) {
				// First-attempt timeout special case: --timeout is shorter
				// than a single API call can complete; the per-poll
				// context fires before the very first GET returns.
				// Rewrite to a friendly message naming the duration.
				//
				// Only fires on attempt 0. On later attempts the first
				// call already succeeded, so saying "shorter than a
				// single API call" would lie — fall through to
				// waitTimeoutError instead.
				if cli.Code == output.ErrTimeout && attempt == 0 {
					return output.NewCLIError(
						output.ErrTimeout,
						fmt.Sprintf("Render %s status check timed out after %s — the --timeout was shorter than a single API call could complete", renderID, f.timeout),
						"Increase --timeout (try 30s or more for --wait), or run a single status check with `urlbox status "+renderID+"` (no --wait).",
					)
				}
				if cli.Code == output.ErrTimeout {
					return waitTimeoutError(renderID, f.timeout, lastStatus)
				}
				return cli
			}
			return output.NewCLIError(output.ErrServer, err.Error(), "Run `urlbox doctor` to verify connectivity.")
		}
		attempt++

		statusStr, _ := resp.Data["status"].(string)
		if statusStr != "" {
			lastStatus = statusStr
		}
		switch statusStr {
		case "succeeded", "failed", "error":
			return writeStatusEnvelope(cmd, resp, renderID)
		}

		// Non-terminal. Decide whether the next poll fits inside --timeout.
		next := statusClock.Now().Add(f.pollInterval)
		if !next.Before(deadline) {
			return waitTimeoutError(renderID, f.timeout, lastStatus)
		}
		statusClock.Sleep(f.pollInterval)
	}
}

// waitTimeoutError builds the ErrTimeout envelope returned when --timeout
// expires before a terminal status is observed. Exit code 11 (timeout
// class) is the agent-friendly classification — a deadline-exceeded
// outcome is operational, not a "bad flag" usage error.
func waitTimeoutError(renderID string, timeout time.Duration, lastStatus string) *output.CLIError {
	if lastStatus == "" {
		lastStatus = "unknown"
	}
	return output.NewCLIError(
		output.ErrTimeout,
		fmt.Sprintf("Render %s timed out after %s (last status: %s)", renderID, timeout, lastStatus),
		"Increase --timeout, or re-run `urlbox status "+renderID+" --wait` later.",
	)
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

	cfg, cfgErr := config.LoadOrCLIError()
	if cfgErr != nil {
		return nil, cfgErr
	}
	profile, _ := cmd.Root().PersistentFlags().GetString("profile")
	overlay, ovErr := loadRepoOverlay()
	if ovErr != nil {
		return nil, ovErr
	}
	resolved, rerr := config.Resolve(config.ResolveOptions{
		FlagAPISecret: f.apiSecret,
		FlagProfile:   profile,
		EnvAPISecret:  os.Getenv(config.EnvAPISecret),
		EnvAPIHost:    os.Getenv(config.EnvAPIHost),
		EnvProfile:    os.Getenv(config.EnvProfile),
		RepoOverlay:   overlay,
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
			"Run `urlbox config path` to find the config file, then check permissions and contents.",
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
