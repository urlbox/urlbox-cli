package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/browser"
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

// renderOpener is the browser-launcher used by --open. Defaults to the
// production OSOpener; tests inject a fake via SetOpenerForTest.
var renderOpener browser.Opener = browser.NewOSOpener()

// SetOpenerForTest swaps in a fake browser.Opener. Pair with t.Cleanup(ResetOpenerForTest).
func SetOpenerForTest(o browser.Opener) { renderOpener = o }

// ResetOpenerForTest restores the production OSOpener.
func ResetOpenerForTest() { renderOpener = browser.NewOSOpener() }

// SetClientForTest swaps in a fake api.Client. Pair with t.Cleanup(ResetClientForTest).
func SetClientForTest(c api.Client) { renderClientOverride = c }

// ResetClientForTest restores production client construction.
func ResetClientForTest() { renderClientOverride = nil }

// SetStdinForTest swaps in an alternate stdin reader. Used to drive `--json -`
// from tests without touching the process's real stdin.
func SetStdinForTest(r io.Reader) { renderStdin = r }

// ResetStdinForTest restores os.Stdin.
func ResetStdinForTest() { renderStdin = os.Stdin }

// defaultRenderTimeout is the per-attempt deadline for a render call. On
// expiry the CLI fails fast with code "timeout" (exit 11) and surfaces the
// three recovery paths via networkHint — it does not retry.
const defaultRenderTimeout = 60 * time.Second

// renderFlags carries every convenience flag the render command supports.
// Booleans here mirror cobra's default-zero pattern; when registering, the
// merge step uses cmd.Flags().Changed(...) to decide whether to write into
// the merged options map.
type renderFlags struct {
	url, format, selector, waitUntil, userAgent, webhookURL string
	width, height, delay, quality                           int
	fullPage, blockAds, darkMode, retina                    bool
	async                                                   bool
	jsonInput                                               string
	preset                                                  string
	output                                                  string
	dryRun, curl, open                                      bool
	noRetry                                                 bool
	maxRetries                                              int
	apiSecret                                               string
	apiSecretStdin                                          bool
	apiSecretFile                                           string
	timeout                                                 time.Duration
}

// flagToOptionKey maps a cobra flag name to the API option key used on the
// wire. Only flags that produce option-map entries appear here; meta flags
// like --dry-run, --curl, --output, --preset are handled separately.
//
// Note: --timeout is a CLI-level duration flag (per-attempt budget); it is
// NOT a render-API option. The API's hard-page-timeout (ms) can still be
// set via --json {"timeout": 30000}.
var flagToOptionKey = map[string]string{
	"format":      "format",
	"width":       "width",
	"height":      "height",
	"full-page":   "full_page",
	"delay":       "delay",
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
	attachRenderFlags(c, f)
	return c
}

// attachRenderFlags registers every render-pipeline flag on c, binding into
// the given *renderFlags. Extracted so the alias commands (screenshot, pdf,
// video) can register the exact same surface and share runRender — there's
// no pipeline duplication.
func attachRenderFlags(c *cobra.Command, f *renderFlags) {
	c.Flags().StringVarP(&f.format, "format", "f", "", "Output format (png, jpeg, pdf, mp4, webp, ...)")
	c.Flags().IntVarP(&f.width, "width", "w", 0, "Viewport width in pixels")
	c.Flags().IntVar(&f.height, "height", 0, "Viewport height in pixels")
	c.Flags().BoolVar(&f.fullPage, "full-page", false, "Capture the full scrollable page")
	c.Flags().IntVar(&f.delay, "delay", 0, "Wait N ms after page load before capture")
	c.Flags().DurationVar(&f.timeout, "timeout", defaultRenderTimeout,
		"Per-attempt timeout for the render call (e.g. 60s, 3m). On timeout, "+
			"the CLI fails fast — it does not retry. Raise this for heavy pages, "+
			"or use --async --webhook-url for very long renders.")
	c.Flags().StringVarP(&f.selector, "selector", "s", "", "CSS selector to capture")
	c.Flags().IntVarP(&f.quality, "quality", "q", 0, "Output quality (1-100, format-dependent)")
	c.Flags().BoolVar(&f.blockAds, "block-ads", false, "Block ads via uBlock filterlists")
	c.Flags().BoolVar(&f.darkMode, "dark-mode", false, "Force prefers-color-scheme: dark")
	c.Flags().BoolVar(&f.retina, "retina", false, "2x device pixel ratio")
	c.Flags().StringVar(&f.waitUntil, "wait-until", "",
		"Page-ready signal: "+api.EnumValuesFor("wait_until"))
	c.Flags().StringVar(&f.userAgent, "user-agent", "", "Override the browser User-Agent")
	c.Flags().StringVar(&f.webhookURL, "webhook-url", "", "POST the result to this URL when async render completes")
	c.Flags().BoolVar(&f.async, "async", false, "Queue the render and return a renderId")
	c.Flags().StringVar(&f.jsonInput, "json", "", "JSON payload (string, '-' for stdin, or '@path' for file)")
	c.Flags().StringVar(&f.preset, "preset", "", "Apply a built-in preset (article, desktop, mobile, pdf-a4)")
	c.Flags().StringVarP(&f.output, "output", "o", "", "Save the rendered file to this path (sandboxed to CWD)")
	c.Flags().BoolVar(&f.dryRun, "dry-run", false, "Validate the merged payload without calling the API")
	c.Flags().BoolVar(&f.curl, "curl", false, "Print an equivalent curl command instead of calling the API")
	c.Flags().BoolVar(&f.open, "open", false, "Open the rendered URL in the default browser after success")
	c.Flags().BoolVar(&f.noRetry, "no-retry", false, "Disable automatic retries on 429 / 5xx")
	c.Flags().IntVar(&f.maxRetries, "max-retries", api.DefaultRetryConfig().MaxRetries, "Maximum retry attempts on 429 / 5xx")
	c.Flags().StringVar(&f.apiSecret, "api-secret", "", "Per-call override of the API secret (leaks via ps + shell history; prefer --api-secret-stdin or --api-secret-file)")
	c.Flags().BoolVar(&f.apiSecretStdin, "api-secret-stdin", false, "Read the API secret from stdin until EOF")
	c.Flags().StringVar(&f.apiSecretFile, "api-secret-file", "", "Read the API secret from the given file (trailing newline trimmed)")
}

func runRender(cmd *cobra.Command, args []string, f *renderFlags) error {
	if len(args) == 1 {
		f.url = args[0]
	}

	// Round 5 Adv-5: reject negative --timeout up front. Without this,
	// `--timeout -5s` produced the nonsense diagnostic "Render timed out
	// after -5s" — the per-attempt context immediately expired and the
	// timeout-error path interpolated the negative duration verbatim.
	if f.timeout < 0 {
		return output.NewCLIError(
			output.ErrUsage,
			fmt.Sprintf("--timeout must be a positive duration; got %s", f.timeout),
			"Pass a positive duration like --timeout 30s or --timeout 2m. Omit the flag to use the default.",
		)
	}

	// Both --api-secret-stdin and `--json -` consume stdin; at most one wins.
	if f.apiSecretStdin && f.jsonInput == "-" {
		return output.NewCLIError(
			output.ErrUsage,
			"--api-secret-stdin and --json - both want stdin",
			"Pipe the secret via --api-secret-file <path> (or use URLBOX_API_SECRET) and reserve stdin for --json.",
		)
	}

	if resolved, cliErr := resolveAPISecretInput(secretStdin, cmd.ErrOrStderr(), f.apiSecret, cmd.Flags().Changed("api-secret"), f.apiSecretStdin, f.apiSecretFile); cliErr != nil {
		return cliErr
	} else if resolved != "" {
		f.apiSecret = resolved
	}

	// Round 4 M2: reject numeric flags outside the JSON safe-int range
	// (±2^53-1). Go int64 accepts values JSON marshalling silently rounds
	// to a nearby float64, so the payload that reaches the API differs
	// from what the user typed. Catch it locally with a precise error.
	if cliErr := validateIntFlagRanges(f); cliErr != nil {
		return cliErr
	}

	// 1. Parse --json source (flag value, stdin, or @file).
	jsonBytes, perr := parseJSONFlag(f.jsonInput, renderStdin)
	if perr != nil {
		return perr
	}

	// 2. Build the merged options map. Merge order is preset < json < flags
	// (last writer wins), so a preset establishes defaults that any later
	// layer can override.
	merged := map[string]any{}

	// 2a. Apply --preset (lowest precedence; defaults that anything overrides).
	if f.preset != "" {
		p := applyPreset(f.preset)
		if p == nil {
			return output.NewCLIError(
				output.ErrUsage,
				"Unknown preset: "+f.preset,
				"Available presets: "+availablePresetNames()+". See `urlbox render --help`.",
			)
		}
		for k, v := range p {
			merged[k] = v
		}
	}

	// 2b. Apply --json contents (overrides preset).
	if len(jsonBytes) > 0 {
		fromJSON, cliErr := parseJSONWithIntRangeCheck(jsonBytes)
		if cliErr != nil {
			return cliErr
		}
		for k, v := range fromJSON {
			merged[k] = v
		}
	}

	// 3. Apply flag overrides (last writer wins). Only flags the user
	// actually passed land in the map — otherwise we'd zero out values
	// that came from --json or a preset.
	applyFlagsToMap(cmd, f, merged)

	// 4. Require url somewhere — and require it to be a non-empty string.
	// Round 5 Adv-7: --json '{"url":""}' and --json '{"url":null}' used to
	// bypass this check because it only verified the key's presence.
	urlVal, urlPresent := merged["url"]
	missing := !urlPresent
	if !missing {
		if urlVal == nil {
			missing = true
		} else if s, ok := urlVal.(string); ok && s == "" {
			missing = true
		}
	}
	if missing {
		return output.NewCLIError(
			output.ErrUsage,
			"missing required url",
			"Provide a positional URL (urlbox render <url>) or include a non-empty \"url\" in --json.",
		)
	}

	// 4.5. Typed-flag enum validation (v0.9.0 schema-as-docs contract).
	// Enforces enum values for typed flags only — the same values supplied
	// via --json passthrough are NOT gated here (the API decides). This is
	// the explicit split: typed flags get fast local feedback; --json is a
	// passthrough.
	for _, ec := range []enumFlagCheck{
		{schemaField: "format", flagName: "--format", value: f.format},
		{schemaField: "wait_until", flagName: "--wait-until", value: f.waitUntil},
	} {
		if cliErr := validateEnumFlag(ec); cliErr != nil {
			return cliErr
		}
	}

	// 5. Validate the merged payload. Schema-as-documentation contract
	// (v0.9.0): only local hard errors are size cap / malformed JSON /
	// control chars / build defects. Unknown keys + bad types pass through
	// to the API; fuzzy-matchable typos emit a stderr warning but the
	// request still goes out verbatim.
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return output.NewCLIError(output.ErrUsage, "failed to encode merged payload", err.Error())
	}
	validated, warnings, vErr := validation.ValidatePayload(mergedJSON)
	if vErr != nil {
		return vErr
	}
	for _, w := range warnings {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
	}

	// 5.5. Validate --profile (and other resolver inputs) BEFORE the
	// dry-run / curl shortcuts. Scripts using --dry-run for pre-flight
	// validation expect a bad profile name to error here, not silently
	// succeed. Missing API secret is tolerated for dry-run / curl —
	// validateResolverFlags only flags the profile-not-found case.
	if cliErr := validateResolverFlags(cmd, f); cliErr != nil {
		return cliErr
	}

	// 5.6. Pre-flight --output sandbox + writability before any API call
	// (Round 4 M1, M6). Without this, --dry-run silently passed paths
	// outside CWD and real renders burned a credit before discovering
	// the target was unwritable. Skipped for --async since --output is
	// not honored on the async path (the renderId is returned instead).
	if f.output != "" && !f.async {
		cwd, _ := os.Getwd()
		abs, cerr := resolveOutputPath(f.output, cwd)
		if cerr != nil {
			return cerr
		}
		if cerr := checkOutputWritable(abs); cerr != nil {
			return cerr
		}
	}

	// 6. --dry-run short-circuits with the validated payload in the envelope.
	if f.dryRun {
		// Round 5 Adv-3: flag the silent precedence when --dry-run wins
		// over --curl / --output. Without this the user has no signal
		// that their --curl print / --output save wasn't performed.
		breadcrumbs := []output.Breadcrumb{
			{Action: "send", Cmd: "urlbox render <url>"},
		}
		if f.curl {
			breadcrumbs = append(breadcrumbs, output.Breadcrumb{
				Action: "ignored",
				Cmd:    "--curl was dropped because --dry-run wins; re-run without --dry-run to print the curl command",
			})
		}
		if f.output != "" {
			breadcrumbs = append(breadcrumbs, output.Breadcrumb{
				Action: "ignored",
				Cmd:    "--output " + f.output + " was dropped because --dry-run wins; re-run without --dry-run to save the file",
			})
		}
		env := output.NewEnvelope(
			"render",
			validated,
			"Dry run: payload validated, no API call made",
			breadcrumbs,
		)
		return writeRenderEnvelope(cmd, env)
	}

	// 7. --curl short-circuits with a copy-pasteable curl command. Secret
	// is redacted as $URLBOX_API_SECRET (never printed).
	if f.curl {
		path := api.PathSync
		if f.async {
			path = api.PathAsync
		}
		host := api.ResolveAPIHost()
		env := output.NewEnvelope(
			"render",
			map[string]any{"curl": buildCurl(host, path, validated)},
			"Equivalent curl command (no API call made)",
			[]output.Breadcrumb{
				{Action: "run", Cmd: "urlbox render <url>"},
			},
		)
		return writeRenderEnvelope(cmd, env)
	}

	// 8. Real call. Build the client (test override wins) and dispatch.
	client, cerr := buildRenderClient(cmd, f)
	if cerr != nil {
		return cerr
	}

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	var resp *api.Response
	if f.async {
		resp, err = client.RenderAsync(ctx, validated)
	} else {
		resp, err = client.Render(ctx, validated)
	}
	if err != nil {
		// Refine the hint for timeout/network errors with the render-aware
		// recovery paths and the actual --timeout value.
		var cli *output.CLIError
		if errors.As(err, &cli) {
			if cli.Code == output.ErrTimeout || cli.Code == output.ErrNetwork {
				cli.Hint = networkHint(errors.New(cli.Message), true, f.timeout)
			}
		}
		return err
	}

	// 9. --output: download the rendered file to a sandboxed local path.
	// Sandbox checks happen BEFORE the download so a malicious path is
	// rejected without burning network round-trips.
	if f.output != "" && !f.async {
		cwd, _ := os.Getwd()
		abs, cerr := resolveOutputPath(f.output, cwd)
		if cerr != nil {
			return cerr
		}
		renderURL, _ := resp.Data["renderUrl"].(string)
		if renderURL == "" {
			return output.NewCLIError(
				output.ErrServer,
				"API response missing renderUrl",
				"The render succeeded but the response had no downloadable URL. Check the dashboard.",
			)
		}
		if cerr := downloadTo(ctx, renderURL, abs); cerr != nil {
			return cerr
		}
		resp.Data["savedTo"] = abs
	}

	// 10. --open: launch the rendered URL in the default browser. Best-effort —
	// failures are non-fatal because the URL is already in the envelope.
	if f.open && !f.async {
		if u, ok := resp.Data["renderUrl"].(string); ok && u != "" {
			_ = renderOpener.Open(u)
		}
	}

	env := output.NewEnvelope(
		"render",
		resp.Data,
		summariseRenderResp(resp),
		breadcrumbsForResp(resp, f),
	)
	return writeRenderEnvelope(cmd, env)
}

// breadcrumbsForResp returns the right next-step breadcrumbs based on what
// the user just did (sync vs async, --output present, etc).
func breadcrumbsForResp(resp *api.Response, f *renderFlags) []output.Breadcrumb {
	if f.async {
		id, _ := resp.Data["renderId"].(string)
		return []output.Breadcrumb{
			{Action: "status", Cmd: "urlbox status " + id},
		}
	}
	// Sync success. Suggest --output if they didn't already use it.
	if f.output == "" {
		return []output.Breadcrumb{
			{Action: "save", Cmd: "urlbox render <url> --output screenshot.png"},
		}
	}
	return nil
}

// parseJSONFlag interprets --json's value:
//   - ""        → no JSON source (return nil bytes, nil error)
//   - "-"       → read all of stdin
//   - "@path"   → read the file at path
//   - anything else → treat as a literal JSON string
//
// jsonSafeIntMax is 2^53 - 1 — the largest integer that JSON's float64
// representation can round-trip exactly. Anything larger silently rounds
// to a nearby even number when marshaled. Schema documents this as the
// width/height/etc. maximum.
const jsonSafeIntMax = 9007199254740991

// parseJSONWithIntRangeCheck unmarshals the --json payload with json.Number
// (string-typed numbers) so we can reject silently-rounding ints BEFORE
// they become float64. Then it converts the surviving numbers back to
// float64 to preserve the existing map[string]any contract downstream.
//
// Round 6 class-fix: walks the ENTIRE JSON tree (nested maps + arrays).
// The previous version only checked top-level keys width/height/delay/
// quality, leaving every other integer — including legitimate fields
// like {"viewport":{"width":...}} — unchecked. The fundamental rule is
// "no integer anywhere should silently round when marshaled to JSON",
// independent of which key it sits under.
func parseJSONWithIntRangeCheck(jsonBytes []byte) (map[string]any, *output.CLIError) {
	// Round 8 LL: detect duplicate keys before main parse. Standard
	// json.Unmarshal silently last-wins on dup keys, which Adv-2 found
	// in `--json '{"url":"a","url":"b"}'` (signed b, no warning). Fail
	// fast so hand-edited JSON typos surface as errors.
	if cliErr := checkDuplicateJSONKeys(jsonBytes); cliErr != nil {
		return nil, cliErr
	}
	raw := map[string]any{}
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, output.NewCLIError(
			output.ErrValidation,
			"failed to parse --json payload",
			"--json accepts a literal JSON string, '-' (stdin), or '@path' (file). Got: "+err.Error(),
		)
	}
	if cliErr := walkAndCheckInts(raw, ""); cliErr != nil {
		return nil, cliErr
	}
	convertNumbersToFloat64(raw)
	return raw, nil
}

// checkDuplicateJSONKeys walks the JSON tree using json.Decoder.Token
// and reports any duplicate keys within the same object. Arrays don't
// have keys, so the dup-check is only relevant inside object scopes.
// Returns nil if no duplicates, *CLIError on the first one found.
//
// On parse error here we return nil — the main parser below will
// produce a more helpful error message, so don't double-surface.
func checkDuplicateJSONKeys(jsonBytes []byte) *output.CLIError {
	dec := json.NewDecoder(bytes.NewReader(jsonBytes))
	// Stack of per-object key sets. Nil entry = current scope is an
	// array (no key dups possible). Path string is dotted-key path
	// for error messages.
	type frame struct {
		isObject bool
		keys     map[string]bool
		path     string
	}
	var stack []frame
	// "Expecting key" is true when the next string token in an object
	// scope is a key (vs a value). Reset after each key→value pair.
	expectingKey := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{':
				path := ""
				if len(stack) > 0 {
					path = stack[len(stack)-1].path
				}
				stack = append(stack, frame{isObject: true, keys: map[string]bool{}, path: path})
				expectingKey = true
			case '}':
				stack = stack[:len(stack)-1]
				// After closing an object, the parent scope continues.
				// If parent is an object, we're between key:value pairs
				// → next token is a key. If parent is an array, next is
				// another array element (value, not key).
				expectingKey = len(stack) > 0 && stack[len(stack)-1].isObject
			case '[':
				path := ""
				if len(stack) > 0 {
					path = stack[len(stack)-1].path
				}
				stack = append(stack, frame{isObject: false, path: path})
				expectingKey = false
			case ']':
				stack = stack[:len(stack)-1]
				expectingKey = len(stack) > 0 && stack[len(stack)-1].isObject
			}
		case string:
			if expectingKey && len(stack) > 0 && stack[len(stack)-1].isObject {
				key := v
				top := stack[len(stack)-1]
				if top.keys[key] {
					location := "--json"
					if top.path != "" {
						location = "--json at " + top.path
					}
					return output.NewCLIError(
						output.ErrValidation,
						fmt.Sprintf("%s contains duplicate key %q", location, key),
						"JSON objects must have unique keys. The standard parser would silently take the last value, which is almost certainly not what the writer intended — fix the payload.",
					)
				}
				top.keys[key] = true
				// Update path so nested objects can report e.g. "viewport.width"
				if top.path == "" {
					top.path = key
				} else {
					top.path += "." + key
				}
				stack[len(stack)-1] = top
				expectingKey = false // next token is the value
			} else if len(stack) > 0 && stack[len(stack)-1].isObject {
				// Just consumed a string VALUE inside an object — next is
				// either another key or '}'.
				expectingKey = true
				trimLastPathSegment(&stack[len(stack)-1].path)
			}
		default:
			// Number / bool / null — value token. In an object, the
			// next token is either another key or '}'.
			if len(stack) > 0 && stack[len(stack)-1].isObject {
				expectingKey = true
				trimLastPathSegment(&stack[len(stack)-1].path)
			}
		}
	}
}

// trimLastPathSegment strips the trailing ".key" component we appended
// when we saw the key. Called after each value so the next key in the
// same object doesn't carry the previous key's name in its path.
func trimLastPathSegment(p *string) {
	idx := strings.LastIndex(*p, ".")
	if idx >= 0 {
		*p = (*p)[:idx]
	} else {
		*p = ""
	}
}

// walkAndCheckInts recursively visits every json.Number in the parsed
// payload and rejects any integer outside ±jsonSafeIntMax. Floats pass
// through verbatim (their precision is already <= 2^53 and the API
// validates value-semantic constraints per the schema-as-docs contract).
//
// `path` is dotted key notation (`viewport.width`) with `[N]` for array
// indices (`items[0].width`), echoed in the error message so the user
// can locate the offending value in a non-trivial payload.
func walkAndCheckInts(v any, path string) *output.CLIError {
	switch v := v.(type) {
	case map[string]any:
		// Sort keys so error reports are deterministic when multiple keys
		// are out of range (we surface the first; sorted order makes it
		// stable across runs and across Go map iteration orders).
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			next := k
			if path != "" {
				next = path + "." + k
			}
			if cliErr := walkAndCheckInts(v[k], next); cliErr != nil {
				return cliErr
			}
		}
	case []any:
		for i, elem := range v {
			next := fmt.Sprintf("%s[%d]", path, i)
			if cliErr := walkAndCheckInts(elem, next); cliErr != nil {
				return cliErr
			}
		}
	case json.Number:
		// Need to distinguish "literal integer (with optional minus,
		// digits only, no decimal point or exponent)" from "fractional
		// or scientific". Only literal integers can silently round when
		// they exceed the safe range — fractional/sci floats already
		// have float64 precision and the API validates value semantics.
		//
		// Round 8 LL: the previous version called v.Int64() and treated
		// any error as "pass through". But Int64() errors on TWO cases:
		// fractional numbers AND integers > 2^63. The latter were silent-
		// rounding to floats downstream (Adv-2 demo: width=99999999999999999999
		// → query 1e+20). Now we inspect the literal string and reject
		// out-of-int64 integers explicitly.
		s := string(v)
		if isIntegerLiteral(s) {
			i, err := v.Int64()
			if err != nil {
				// Literal integer that doesn't fit in int64 — definitely
				// >2^63 or <-2^63. Way past our 2^53 limit.
				location := "--json"
				if path != "" {
					location = "--json key \"" + path + "\""
				}
				return output.NewCLIError(
					output.ErrValidation,
					fmt.Sprintf("%s value %s is too large to represent as a JSON-safe integer", location, s),
					"Pass an integer between -9007199254740991 and 9007199254740991. Larger values silently round when marshaled to JSON.",
				)
			}
			if i > jsonSafeIntMax || i < -jsonSafeIntMax {
				location := "--json"
				if path != "" {
					location = "--json key \"" + path + "\""
				}
				return output.NewCLIError(
					output.ErrValidation,
					fmt.Sprintf("%s value %d is outside the JSON safe-integer range (±%d)", location, i, int64(jsonSafeIntMax)),
					"Pass an integer between -9007199254740991 and 9007199254740991. Larger values silently round when marshaled to JSON.",
				)
			}
		}
	}
	return nil
}

// isIntegerLiteral returns true if s is an integer literal (optional
// leading minus, then one or more digits, nothing else). Fractional
// values ("1.5") or scientific notation ("1e10") return false — those
// are floats by design and don't need the safe-integer check.
func isIntegerLiteral(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i = 1
		if len(s) == 1 {
			return false
		}
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// convertNumbersToFloat64 walks a parsed JSON map (json.Decoder with
// UseNumber leaves numbers as json.Number) and converts each to float64
// so the rest of the pipeline sees the same shape it always has.
// Recurses into nested maps AND slices so nested option objects /
// arrays don't leave stray json.Number values that would render
// inconsistently if printed via the envelope. Non-finite floats
// (NaN/Inf — not valid JSON anyway) fall back to their string form.
func convertNumbersToFloat64(m map[string]any) {
	for k, v := range m {
		m[k] = convertNumberValue(v)
	}
}

func convertNumberValue(v any) any {
	switch v := v.(type) {
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	case map[string]any:
		convertNumbersToFloat64(v)
		return v
	case []any:
		for i, elem := range v {
			v[i] = convertNumberValue(elem)
		}
		return v
	default:
		return v
	}
}

// validateIntFlagRanges rejects --width / --height / --delay / --quality
// / --max-retries values outside ±2^53-1. The Go int64 flag accepts much
// larger; we have to catch it here before json.Marshal silently rounds.
// Round 4 M2.
func validateIntFlagRanges(f *renderFlags) *output.CLIError {
	checks := []struct {
		flag string
		val  int
	}{
		{"--width", f.width},
		{"--height", f.height},
		{"--delay", f.delay},
		{"--quality", f.quality},
		{"--max-retries", f.maxRetries},
	}
	for _, c := range checks {
		if int64(c.val) > jsonSafeIntMax || int64(c.val) < -jsonSafeIntMax {
			return output.NewCLIError(
				output.ErrValidation,
				fmt.Sprintf("%s value %d is outside the JSON safe-integer range (±%d)", c.flag, c.val, int64(jsonSafeIntMax)),
				"Pass a value between -9007199254740991 and 9007199254740991. Larger values would silently round when marshaled to JSON.",
			)
		}
	}
	return nil
}

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

// validateResolverFlags surfaces resolver-side errors (today: only the
// "--profile X doesn't exist" case) BEFORE --dry-run / --curl shortcut. A
// real call would hit the same error via buildRenderClient → config.Resolve;
// the dry-run / curl paths skip client construction entirely, so without
// this pre-check `urlbox render <url> --dry-run --profile bad` would
// silently succeed (false-positive for scripts using --dry-run as
// pre-flight validation).
//
// Missing API secret is intentionally not surfaced here — the user may not
// have a secret configured yet and dry-run should still validate the
// payload shape. Resolve() doesn't error on missing secret; only on
// FlagProfile naming a profile that doesn't exist.
//
// renderClientOverride bypasses this check (tests with the override
// shouldn't need a real config).
func validateResolverFlags(cmd *cobra.Command, f *renderFlags) *output.CLIError {
	if renderClientOverride != nil {
		return nil
	}
	cfg, err := config.Load()
	if err != nil {
		return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
	}
	profile, _ := cmd.Root().PersistentFlags().GetString("profile")
	overlay, ovErr := loadRepoOverlay()
	if ovErr != nil {
		return ovErr
	}
	_, rerr := config.Resolve(config.ResolveOptions{
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
			return cli
		}
		// Non-CLIError shouldn't happen today (Resolve only emits CLIError),
		// but route to a sensible default so we don't silently swallow.
		return output.NewCLIError(output.ErrUsage, rerr.Error(),
			"Run `urlbox config profile list` to inspect, or `urlbox config path` to find the file.")
	}
	return nil
}

// buildRenderClient returns the test-injected client if present, else a
// production HTTPClient resolved from env/config.
func buildRenderClient(cmd *cobra.Command, f *renderFlags) (api.Client, *output.CLIError) {
	if renderClientOverride != nil {
		return renderClientOverride, nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
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
			"Run `urlbox config profile list` to inspect, or set --api-secret / URLBOX_API_SECRET explicitly.",
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

// summariseRenderResp builds the human-readable summary line.
func summariseRenderResp(resp *api.Response) string {
	upstream := ""
	if ok, present := resp.Data["upstreamOk"].(bool); present && !ok {
		if status, _ := resp.Data["upstreamStatus"].(float64); status > 0 {
			upstream = fmt.Sprintf(" (warning: upstream returned HTTP %d — likely captcha / login wall / blocked)", int(status))
		} else {
			upstream = " (warning: upstream returned an error status)"
		}
	}
	if id, ok := resp.Data["renderId"].(string); ok {
		return fmt.Sprintf("Render queued (renderId: %s)%s", id, upstream)
	}
	if u, ok := resp.Data["renderUrl"].(string); ok {
		return "Rendered: " + u + upstream
	}
	return "Render complete" + upstream
}

// writeRenderEnvelope picks the right formatter (json/text/quiet) and
// optionally pipes through --jq before printing.
//
// Quiet mode is bespoke: instead of dumping .data JSON (the generic
// QuietFormatter default), render emits a SINGLE useful scalar so the
// output is pipeline-ready. The chosen field depends on context:
//
//   - sync + --output saved a file → the absolute saved path (savedTo)
//   - sync + no --output → the hosted renderUrl
//   - async → the renderId for follow-up `urlbox status` calls
//
// Round 5 First-3 / Power-1.
func writeRenderEnvelope(cmd *cobra.Command, env *output.Envelope) error {
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	stdout := cmd.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if jqExpr != "" {
		return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
	}
	if format == output.FormatQuiet {
		if primary := quietPrimaryValue(env); primary != "" {
			_, werr := fmt.Fprintln(stdout, primary)
			return werr
		}
		// No obvious primary value (e.g. --dry-run / --curl flows) →
		// fall through to the generic quiet formatter which emits .data.
	}
	styles := output.NewStylesForWriter(stdout)
	return output.NewFormatter(format, styles).WriteSuccess(stdout, env)
}

// quietPrimaryValue picks the single pipeline-ready scalar from a
// render envelope. Returns "" when no obvious primary exists (caller
// falls back to the generic data dump).
//
// Precedence reflects "what did the user most likely want next?":
//   - savedTo (they passed --output; the local file path is the deliverable)
//   - renderUrl (sync render; the hosted asset URL is the deliverable)
//   - renderId (async render; needed for `urlbox status`)
func quietPrimaryValue(env *output.Envelope) string {
	data, ok := env.Data.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"savedTo", "renderUrl", "renderId"} {
		if s, ok := data[key].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// enumFlagCheck pairs a typed-flag's user-facing name with the schema field
// whose enum we enforce against. Used by validateEnumFlag (v0.9.0
// schema-as-docs).
type enumFlagCheck struct {
	schemaField string // e.g. "wait_until"
	flagName    string // e.g. "--wait-until" (user-facing, included in error)
	value       string // current value of the flag (empty = flag not set)
}

// validateEnumFlag returns a validation CLIError when the typed-flag value
// is non-empty and not in the schema's enum list. When the schema doesn't
// publish an enum for the field, the check is a no-op (don't reject what
// we can't validate). Empty values mean the flag wasn't set — also no-op.
//
// Equivalent values supplied via --json are NOT subject to this check; the
// API decides for those (schema-as-docs contract).
func validateEnumFlag(ec enumFlagCheck) *output.CLIError {
	if ec.value == "" {
		return nil
	}
	allowed := api.EnumSliceFor(ec.schemaField)
	if len(allowed) == 0 {
		return nil
	}
	for _, a := range allowed {
		if a == ec.value {
			return nil
		}
	}
	hint := "Allowed: " + strings.Join(allowed, ", ")
	if suggestion, ok := validation.ClosestMatch(ec.value, allowed); ok {
		hint = `Did you mean "` + suggestion + `"? ` + hint
	}
	return output.NewCLIError(
		output.ErrValidation,
		ec.flagName+`: invalid value "`+ec.value+`"`,
		hint,
	)
}
