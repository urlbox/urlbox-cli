package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/validation"
)

// renderClientOverride is a test injection point. Production builds always
// see nil and construct a real api.HTTPClient.
var renderClientOverride api.Client

// renderStdin is the source for `--json -` reads. Defaults to os.Stdin;
// tests inject a strings.Reader via SetStdinForTest.
var renderStdin io.Reader = os.Stdin

// SetClientForTest swaps in a fake api.Client. Pair with t.Cleanup(ResetClientForTest).
func SetClientForTest(c api.Client) { renderClientOverride = c }

// ResetClientForTest restores production client construction.
func ResetClientForTest() { renderClientOverride = nil }

// SetStdinForTest swaps in an alternate stdin reader. Used to drive `--json -`
// from tests without touching the process's real stdin.
func SetStdinForTest(r io.Reader) { renderStdin = r }

// ResetStdinForTest restores os.Stdin.
func ResetStdinForTest() { renderStdin = os.Stdin }

// renderFlags carries every convenience flag the render command supports.
// Booleans here mirror cobra's default-zero pattern; when registering, the
// merge step uses cmd.Flags().Changed(...) to decide whether to write into
// the merged options map.
type renderFlags struct {
	url, format, selector, waitUntil, userAgent, webhookURL string
	width, height, delay, timeout, quality                  int
	fullPage, blockAds, darkMode, retina                    bool
	async                                                   bool
	jsonInput                                               string
	preset                                                  string
	output                                                  string
	dryRun, curl, open                                      bool
	noRetry                                                 bool
	maxRetries                                              int
	apiSecret                                               string
}

// flagToOptionKey maps a cobra flag name to the API option key used on the
// wire. Only flags that produce option-map entries appear here; meta flags
// like --dry-run, --curl, --output, --preset are handled separately.
var flagToOptionKey = map[string]string{
	"format":      "format",
	"width":       "width",
	"height":      "height",
	"full-page":   "full_page",
	"delay":       "delay",
	"timeout":     "timeout",
	"selector":    "selector",
	"quality":     "quality",
	"block-ads":   "block_ads",
	"dark-mode":   "dark_mode",
	"retina":      "retina",
	"wait-until":  "wait_until",
	"user-agent":  "user_agent",
	"webhook-url": "webhook_url",
}

func newRenderCmd() *cobra.Command {
	f := &renderFlags{}
	c := &cobra.Command{
		Use:   "render [url]",
		Short: "Render a URL to a screenshot, PDF, video, or other format",
		Long: `Render a URL via the Urlbox API.

Provide options three ways (later sources override earlier):
  preset → --json (flag, stdin, or @file) → --flag values

Examples:
  urlbox render https://example.com --format png
  urlbox render --json '{"url":"https://example.com","format":"pdf"}'
  urlbox render --json @opts.json
  cat opts.json | urlbox render --json -

Use --dry-run to validate the merged payload without making an API call.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRender(cmd, args, f)
		},
	}

	c.Flags().StringVarP(&f.format, "format", "f", "", "Output format (png, jpeg, pdf, mp4, webp, ...)")
	c.Flags().IntVarP(&f.width, "width", "w", 0, "Viewport width in pixels")
	c.Flags().IntVar(&f.height, "height", 0, "Viewport height in pixels")
	c.Flags().BoolVar(&f.fullPage, "full-page", false, "Capture the full scrollable page")
	c.Flags().IntVar(&f.delay, "delay", 0, "Wait N ms after page load before capture")
	c.Flags().IntVar(&f.timeout, "timeout", 0, "Hard timeout in ms")
	c.Flags().StringVarP(&f.selector, "selector", "s", "", "CSS selector to capture")
	c.Flags().IntVarP(&f.quality, "quality", "q", 0, "Output quality (1-100, format-dependent)")
	c.Flags().BoolVar(&f.blockAds, "block-ads", false, "Block ads via uBlock filterlists")
	c.Flags().BoolVar(&f.darkMode, "dark-mode", false, "Force prefers-color-scheme: dark")
	c.Flags().BoolVar(&f.retina, "retina", false, "2x device pixel ratio")
	c.Flags().StringVar(&f.waitUntil, "wait-until", "", "Page-ready signal (load, domcontentloaded, networkidle0, ...)")
	c.Flags().StringVar(&f.userAgent, "user-agent", "", "Override the browser User-Agent")
	c.Flags().StringVar(&f.webhookURL, "webhook-url", "", "POST the result to this URL when async render completes")
	c.Flags().BoolVar(&f.async, "async", false, "Queue the render and return a renderId")
	c.Flags().StringVar(&f.jsonInput, "json", "", "JSON payload (string, '-' for stdin, or '@path' for file)")
	c.Flags().StringVar(&f.preset, "preset", "", "Apply a built-in preset (mobile, desktop, pdf-a4)")
	c.Flags().StringVarP(&f.output, "output", "o", "", "Save the rendered file to this path (sandboxed to CWD)")
	c.Flags().BoolVar(&f.dryRun, "dry-run", false, "Validate the merged payload without calling the API")
	c.Flags().BoolVar(&f.curl, "curl", false, "Print an equivalent curl command instead of calling the API")
	c.Flags().BoolVar(&f.open, "open", false, "Open the rendered URL in the default browser after success")
	c.Flags().BoolVar(&f.noRetry, "no-retry", false, "Disable automatic retries on 429 / 5xx")
	c.Flags().IntVar(&f.maxRetries, "max-retries", 3, "Maximum retry attempts on 429 / 5xx")
	c.Flags().StringVar(&f.apiSecret, "api-secret", "", "Per-call override of the API secret (else read from config / env)")

	return c
}

func runRender(cmd *cobra.Command, args []string, f *renderFlags) error {
	if len(args) == 1 {
		f.url = args[0]
	}

	// 1. Parse --json source (flag value, stdin, or @file).
	jsonBytes, perr := parseJSONFlag(f.jsonInput, renderStdin)
	if perr != nil {
		return perr
	}

	// 2. Build the merged options map.
	merged := map[string]any{}
	if len(jsonBytes) > 0 {
		if err := json.Unmarshal(jsonBytes, &merged); err != nil {
			return output.NewCLIError(
				output.ErrUsage,
				"failed to parse --json payload",
				"--json accepts a literal JSON string, '-' (stdin), or '@path' (file). Got: "+err.Error(),
			)
		}
	}

	// 3. Apply flag overrides (last writer wins). Only flags the user
	// actually passed land in the map — otherwise we'd zero out values
	// that came from --json or a future preset.
	applyFlagsToMap(cmd, f, merged)

	// 4. Require url somewhere.
	if _, ok := merged["url"]; !ok {
		return output.NewCLIError(
			output.ErrUsage,
			"missing required url",
			"Provide a positional URL (urlbox render <url>) or include \"url\" in --json.",
		)
	}

	// 5. Validate the merged payload (size cap, control chars, fuzzy
	// correction, JSON Schema). Returns the canonical map.
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return output.NewCLIError(output.ErrUsage, "failed to encode merged payload", err.Error())
	}
	validated, vErr := validation.ValidatePayload(mergedJSON)
	if vErr != nil {
		return vErr
	}

	// 6. --dry-run short-circuits with the validated payload in the envelope.
	if f.dryRun {
		env := output.NewEnvelope(
			"render",
			validated,
			"Dry run: payload validated, no API call made",
			[]output.Breadcrumb{
				{Action: "send", Cmd: "urlbox render <url>"},
			},
		)
		return writeRenderEnvelope(cmd, env)
	}

	// 7. Real call. Build the client (test override wins) and dispatch.
	client, cerr := buildRenderClient(f)
	if cerr != nil {
		return cerr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var resp *api.Response
	if f.async {
		resp, err = client.RenderAsync(ctx, validated)
	} else {
		resp, err = client.Render(ctx, validated)
	}
	if err != nil {
		return err
	}

	env := output.NewEnvelope(
		"render",
		resp.Data,
		summariseRenderResp(resp),
		[]output.Breadcrumb{
			{Action: "open", Cmd: "urlbox dashboard"},
		},
	)
	return writeRenderEnvelope(cmd, env)
}

// parseJSONFlag interprets --json's value:
//   - ""        → no JSON source (return nil bytes, nil error)
//   - "-"       → read all of stdin
//   - "@path"   → read the file at path
//   - anything else → treat as a literal JSON string
func parseJSONFlag(value string, stdin io.Reader) ([]byte, *output.CLIError) {
	switch {
	case value == "":
		return nil, nil
	case value == "-":
		b, err := io.ReadAll(stdin)
		if err != nil {
			return nil, output.NewCLIError(output.ErrUsage, "failed to read --json from stdin", err.Error())
		}
		return b, nil
	case strings.HasPrefix(value, "@"):
		path := value[1:]
		b, err := os.ReadFile(path) //nolint:gosec // path is user-supplied for --json @file; the read scope is intentional
		if err != nil {
			return nil, output.NewCLIError(output.ErrUsage, "failed to read --json file: "+path, err.Error())
		}
		return b, nil
	default:
		return []byte(value), nil
	}
}

// applyFlagsToMap writes flag values into m, but only for flags the user
// actually passed (cmd.Flags().Changed). Unset flags keep whatever came
// from --json or a preset.
func applyFlagsToMap(cmd *cobra.Command, f *renderFlags, m map[string]any) {
	if f.url != "" {
		m["url"] = f.url
	}
	for flagName, optionKey := range flagToOptionKey {
		if !cmd.Flags().Changed(flagName) {
			continue
		}
		switch flagName {
		case "format":
			m[optionKey] = f.format
		case "width":
			m[optionKey] = f.width
		case "height":
			m[optionKey] = f.height
		case "full-page":
			m[optionKey] = f.fullPage
		case "delay":
			m[optionKey] = f.delay
		case "timeout":
			m[optionKey] = f.timeout
		case "selector":
			m[optionKey] = f.selector
		case "quality":
			m[optionKey] = f.quality
		case "block-ads":
			m[optionKey] = f.blockAds
		case "dark-mode":
			m[optionKey] = f.darkMode
		case "retina":
			m[optionKey] = f.retina
		case "wait-until":
			m[optionKey] = f.waitUntil
		case "user-agent":
			m[optionKey] = f.userAgent
		case "webhook-url":
			m[optionKey] = f.webhookURL
		}
	}
}

// buildRenderClient returns the test-injected client if present, else a
// production HTTPClient resolved from env/config.
func buildRenderClient(f *renderFlags) (api.Client, *output.CLIError) {
	if renderClientOverride != nil {
		return renderClientOverride, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
	}
	resolved, rerr := config.Resolve(config.ResolveOptions{
		FlagAPISecret: f.apiSecret,
		EnvAPISecret:  os.Getenv(config.EnvAPISecret),
		EnvAPIHost:    os.Getenv(config.EnvAPIHost),
		Config:        cfg,
	})
	if rerr != nil {
		var cli *output.CLIError
		if asCLIError(rerr, &cli) {
			return nil, cli
		}
		return nil, output.NewCLIError(output.ErrUsage, rerr.Error(), "")
	}

	host := resolved.APIHost
	if host == "" {
		host = api.ResolveAPIHost()
	}

	c := api.NewHTTPClient(host, resolved.APIKey, resolved.APISecret)
	if f.noRetry {
		c.Retry = api.NoRetryConfig()
	} else if f.maxRetries != 3 {
		c.Retry.MaxRetries = f.maxRetries
	}
	return c, nil
}

// asCLIError is a thin wrapper around errors.As to keep call sites compact.
func asCLIError(err error, target **output.CLIError) bool {
	for cur := err; cur != nil; {
		if cli, ok := cur.(*output.CLIError); ok {
			*target = cli
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := cur.(unwrapper)
		if !ok {
			break
		}
		cur = u.Unwrap()
	}
	return false
}

// summariseRenderResp builds the human-readable summary line.
func summariseRenderResp(resp *api.Response) string {
	if id, ok := resp.Data["renderId"].(string); ok {
		return fmt.Sprintf("Render queued (renderId: %s)", id)
	}
	if u, ok := resp.Data["renderUrl"].(string); ok {
		return "Rendered: " + u
	}
	return "Render complete"
}

// writeRenderEnvelope picks the right formatter (json/text/quiet) and
// optionally pipes through --jq before printing.
func writeRenderEnvelope(cmd *cobra.Command, env *output.Envelope) error {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	stdout := cmd.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if jqExpr != "" {
		return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
	}
	styles := output.NewStylesForWriter(stdout)
	return output.NewFormatter(format, styles).WriteSuccess(stdout, env)
}
