// internal/cmd/link.go
package cmd

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/validation"
)

// linkFlags carries every convenience flag the link command supports.
type linkFlags struct {
	urlFlag        string
	jsonInput      string
	formatFlag     string
	apiKey         string
	apiSecret      string
	apiSecretStdin bool
	apiSecretFile  string
}

func newLinkCmd() *cobra.Command {
	f := &linkFlags{}
	c := &cobra.Command{
		Use:   "link [url]",
		Short: "Generate an HMAC-SHA256 signed render URL (no API call)",
		Long: `Generate an HMAC-SHA256 signed render URL of the form
  https://api.urlbox.com/v1/<api_key>/<token>/<format>?<encoded_options>

Pure local computation — no API call is made. Requires both the publishable
API key and the API secret.

Specify the target URL via positional argument or --url (the flag wins
on conflict):

  urlbox link https://example.com
  urlbox link --url https://example.com

For pipelines, --output-format quiet emits the bare signed URL on
stdout (no envelope), e.g.:

  urlbox link https://example.com --output-format quiet

If you actually want the rendered asset, use:
  urlbox render --json '{"url":"..."}'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLink(cmd, args, f)
		},
	}
	c.Flags().StringVar(&f.urlFlag, "url", "", "URL to render (required unless --json provides one)")
	c.Flags().StringVar(&f.jsonInput, "json", "", `JSON options payload (literal, '-' for stdin, or '@path' for file)`)
	c.Flags().StringVarP(&f.formatFlag, "format", "f", "", "Output format (default: png, or whatever --json sets)")
	c.Flags().StringVar(&f.apiKey, "api-key", "", "Override resolved API key")
	c.Flags().StringVar(&f.apiSecret, "api-secret", "", "Override resolved API secret (leaks via ps + shell history; prefer --api-secret-stdin or --api-secret-file)")
	c.Flags().BoolVar(&f.apiSecretStdin, "api-secret-stdin", false, "Read the API secret from stdin until EOF")
	c.Flags().StringVar(&f.apiSecretFile, "api-secret-file", "", "Read the API secret from the given file (trailing newline trimmed)")
	return c
}

func runLink(cmd *cobra.Command, args []string, f *linkFlags) error {
	// Round 5 First-1: accept a positional URL like `render` does so
	// `urlbox link https://example.com` is the obvious entry point. --url
	// still wins when both are present, matching render's precedence.
	if len(args) == 1 && f.urlFlag == "" {
		f.urlFlag = args[0]
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

	// 1. Parse --json source (flag value, stdin, or @file). Reuses render's
	// helper so the parsing surface is one and the same.
	jsonBytes, perr := parseJSONFlag(f.jsonInput, renderStdin)
	if perr != nil {
		return perr
	}

	options := map[string]any{}
	if len(jsonBytes) > 0 {
		if err := json.Unmarshal(jsonBytes, &options); err != nil {
			return output.NewCLIError(
				output.ErrValidation,
				"failed to parse --json payload",
				`--json accepts a literal JSON string, '-' (stdin), or '@path' (file). Got: `+err.Error(),
			)
		}
	}

	// 2. Layer flags on top (last writer wins).
	if f.urlFlag != "" {
		options["url"] = f.urlFlag
	}
	if f.formatFlag != "" {
		options["format"] = f.formatFlag
	}

	// 3. URL is required.
	if _, ok := options["url"]; !ok {
		return output.NewCLIError(
			output.ErrUsage,
			`Missing required option "url"`,
			`Pass --url <url> or --json '{"url":"..."}'`,
		)
	}

	// 3a. Sanitize the URL field — control characters in a URL are an
	// unambiguous validation error and must never reach the signer.
	if s, ok := options["url"].(string); ok {
		if cliErr := validation.SanitizeStringField("url", s); cliErr != nil {
			return cliErr
		}
	}

	// 4. Resolve credentials. Flag overrides win at the resolver layer; we
	// also fall back to env (URLBOX_API_SECRET) so users with the env var
	// already set don't need to pass --api-secret.
	cfg, err := config.Load()
	if err != nil {
		return output.NewCLIError(output.ErrServer, "failed to read config", err.Error())
	}
	profile, _ := cmd.Root().PersistentFlags().GetString("profile")
	resolved, rerr := config.Resolve(config.ResolveOptions{
		FlagAPIKey:    f.apiKey,
		FlagAPISecret: f.apiSecret,
		FlagProfile:   profile,
		EnvAPISecret:  os.Getenv(config.EnvAPISecret),
		EnvProfile:    os.Getenv(config.EnvProfile),
		EnvAPIHost:    os.Getenv(config.EnvAPIHost),
		Config:        cfg,
	})
	if rerr != nil {
		// Surface profile-not-found (a CLIError from the resolver) as-is so
		// the upstream code/hint reach the user; otherwise wrap.
		var cli *output.CLIError
		if errors.As(rerr, &cli) {
			return cli
		}
		return output.NewCLIError(output.ErrUsage, rerr.Error(), "Run `urlbox config path` to find the config file, then check permissions and contents.")
	}

	if resolved.APIKey == "" {
		// Round 5 First-1: the previous hint referenced `urlbox auth`
		// which doesn't take --api-key — sent users down a dead end.
		// Both surviving suggestions actually set api_key.
		return output.NewCLIError(
			output.ErrAuth,
			"Missing publishable API key",
			"Pass --api-key <key>, run `urlbox config set api_key <key>`, or include --api-key when creating a profile via `urlbox config profile create <name> --api-key <key>`. The publishable key is on your project dashboard, distinct from the API secret.",
		)
	}
	if resolved.APISecret == "" {
		return output.NewCLIError(
			output.ErrAuth,
			"Missing API secret",
			"Pass --api-secret, set URLBOX_API_SECRET, or run `urlbox auth`. "+
				"`urlbox link` cannot sign without the secret.",
		)
	}

	apiHost := resolved.APIHost
	if apiHost == "" {
		apiHost = api.ResolveAPIHost()
	}

	signed, formatUsed, qs, token := signRenderURL(apiHost, resolved.APIKey, resolved.APISecret, options)

	// 5. Pick formatter. Quiet mode is bespoke for link: emit the bare signed
	// URL with no envelope and no JSON quoting, so it can be piped into
	// `xargs curl` or similar.
	formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
	jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
	stdout := cmd.OutOrStdout()
	format := output.ResolveFormat(formatFlag, stdout)
	if format == output.FormatQuiet && jqExpr == "" {
		_, werr := fmt.Fprintln(stdout, signed)
		return werr
	}

	data := map[string]any{
		"url":    signed,
		"token":  token,
		"key":    resolved.APIKey,
		"format": formatUsed,
		"query":  qs,
	}
	urlValue, _ := options["url"].(string)
	summary := fmt.Sprintf("Signed render URL for %s (%s)", urlValue, formatUsed)
	breadcrumbs := []output.Breadcrumb{
		{
			Action: "render",
			Cmd:    fmt.Sprintf(`urlbox render --json '%s'`, mustJSON(options)),
		},
	}
	env := output.NewEnvelope("link", data, summary, breadcrumbs)
	return writeEnvelope(cmd, env)
}

// signRenderURL builds the canonical signed render URL. Pure function.
// Returns (fullURL, formatUsed, canonicalQueryString, hmacToken).
func signRenderURL(apiHost, apiKey, apiSecret string, options map[string]any) (full, format, qs, token string) {
	format = "png"
	if v, ok := options["format"]; ok {
		if s, ok := v.(string); ok && s != "" {
			format = s
		}
	}

	qs = canonicalQueryString(options)
	token = hmacToken(apiSecret, qs)

	full = apiHost + "/v1/" + apiKey + "/" + token + "/" + format
	if qs != "" {
		full = full + "?" + qs
	}
	return full, format, qs, token
}

// hmacToken returns the lowercase hex HMAC-SHA256 of qs keyed by apiSecret.
func hmacToken(apiSecret, qs string) string {
	mac := hmac.New(sha256.New, []byte(apiSecret))
	_, _ = mac.Write([]byte(qs))
	return hex.EncodeToString(mac.Sum(nil))
}

// canonicalQueryString returns the deterministic url-encoded query string,
// sorted alphabetically by key (then by value for repeated keys), excluding
// the "format" key (which lives in the URL path component).
//
// Encoding note: Go's url.Values.Encode percent-encodes per RFC 3986 *but*
// uses '+' for spaces in query values (matching the application/x-www-form-
// urlencoded form). The fixture digests in link_test.go assume that exact
// behaviour — re-derive the digests if the encoder ever changes.
func canonicalQueryString(options map[string]any) string {
	values := url.Values{}
	for k, v := range options {
		if k == "format" {
			continue
		}
		switch vv := v.(type) {
		case string:
			values.Add(k, vv)
		case bool:
			values.Add(k, fmt.Sprintf("%t", vv))
		case float64:
			// JSON numbers always decode as float64. Render as integer when
			// safe so width=1920 doesn't become width=1920.000000.
			if vv == float64(int64(vv)) {
				values.Add(k, fmt.Sprintf("%d", int64(vv)))
			} else {
				values.Add(k, fmt.Sprintf("%v", vv))
			}
		case []any:
			// Multi-value: stringify each, sort, append.
			strs := make([]string, 0, len(vv))
			for _, item := range vv {
				strs = append(strs, fmt.Sprintf("%v", item))
			}
			sort.Strings(strs)
			for _, s := range strs {
				values.Add(k, s)
			}
		case nil:
			values.Add(k, "")
		default:
			// Nested object / other — JSON-encode as a fallback.
			b, _ := json.Marshal(vv)
			values.Add(k, string(b))
		}
	}
	return values.Encode() // sorts by key.
}

// mustJSON marshals v to a compact JSON string. For the link breadcrumb,
// this is best-effort: an unrepresentable value yields "" rather than a
// crash, but every value in our options map is JSON-derived already.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.ReplaceAll(string(b), "'", `'\''`)
}
