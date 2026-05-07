package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// Endpoint paths locked from urlbox-mono spec
// (apps/api/src/modules/render/render.routes.ts). Exported so callers like
// `urlbox render --curl` can reference the same paths the HTTPClient uses.
const (
	// PathSync is the synchronous render endpoint.
	PathSync = "/v1/screenshot"
	// PathAsync is the asynchronous render endpoint (returns 201 + renderId).
	PathAsync = "/v1/screenshot/async"
	// PathStatus is the prefix for render-status lookups; append the
	// URL-escaped renderID.
	PathStatus = "/v1/render/"
)

// HTTPClient is the production api.Client implementation.
type HTTPClient struct {
	BaseURL   string
	APIKey    string // publishable key (reserved for HMAC URL signing in Phase 5)
	APISecret string // secret key for the Authorization: Bearer header
	UserAgent string
	Timeout   time.Duration
	Retry     RetryConfig
	HTTP      *http.Client // injectable for tests; default is built from Timeout
}

// NewHTTPClient constructs a client with spec defaults. baseURL must be
// non-empty (caller resolves URLBOX_API_HOST + config + flags).
func NewHTTPClient(baseURL, apiKey, apiSecret string) *HTTPClient {
	timeout := 30 * time.Second
	return &HTTPClient{
		BaseURL:   baseURL,
		APIKey:    apiKey,
		APISecret: apiSecret,
		UserAgent: BuildUserAgent(version.Version),
		Timeout:   timeout,
		Retry:     DefaultRetryConfig(),
		HTTP: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// Render performs a synchronous render. POST /v1/screenshot.
func (c *HTTPClient) Render(ctx context.Context, opts map[string]any) (*Response, error) {
	return c.do(ctx, http.MethodPost, PathSync, opts)
}

// RenderAsync queues a render. POST /v1/screenshot/async. Returns 201 with
// {status, renderId, statusUrl}.
func (c *HTTPClient) RenderAsync(ctx context.Context, opts map[string]any) (*Response, error) {
	return c.do(ctx, http.MethodPost, PathAsync, opts)
}

// Status returns the latest state of an async render. GET /v1/render/<renderID>.
func (c *HTTPClient) Status(ctx context.Context, renderID string) (*Response, error) {
	return c.do(ctx, http.MethodGet, PathStatus+url.PathEscape(renderID), nil)
}

// do is the single request entry point: build, send (through retry), parse.
func (c *HTTPClient) do(ctx context.Context, method, path string, body any) (*Response, error) {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, output.NewCLIError(output.ErrUsage, "failed to encode request body", err.Error())
		}
		bodyBytes = b
	}

	send := func() (*http.Response, error) {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
		if err != nil {
			return nil, err
		}
		if c.APISecret != "" {
			req.Header.Set("Authorization", "Bearer "+c.APISecret)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.UserAgent)
		return c.HTTP.Do(req)
	}

	resp, err := RetryDo(ctx, c.Retry, send)
	if err != nil {
		// Classify a deadline as ErrTimeout so retry.go can short-circuit
		// AND the render command can refine the hint with render-specific text.
		code := output.ErrNetwork
		if errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			code = output.ErrTimeout
		}
		return nil, output.NewCLIError(
			code,
			err.Error(),
			"Check your internet connection and the API host (URLBOX_API_HOST). Run `urlbox doctor`.",
		)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, output.NewCLIError(output.ErrNetwork, readErr.Error(), "Run `urlbox doctor` to verify connectivity.")
	}

	if resp.StatusCode >= 400 {
		return nil, mapStatusToCLIError(resp, respBody)
	}

	// Success path: the API returns the response body directly (no envelope
	// wrapper). Parse into a map and synthesize a Response with OK=true.
	data := map[string]any{}
	if len(respBody) > 0 {
		if err := json.Unmarshal(respBody, &data); err != nil {
			return nil, output.NewCLIError(output.ErrServer, "failed to parse API response", err.Error())
		}
	}

	// If the API surfaced the upstream HTTP status (under data.response per
	// urlbox-mono apps/api/src/lib/utils.ts:86-122), promote it to top-level
	// agent-friendly fields. statusCodeInitial captures the first request's
	// code before redirects — a 401→302→200 chain must mark upstreamOk=false.
	if respObj, ok := data["response"].(map[string]any); ok {
		if status, ok := respObj["statusCode"].(float64); ok {
			data["upstreamStatus"] = status
			ok2xx3xx := status >= 200 && status < 400
			if initial, ok := respObj["statusCodeInitial"].(float64); ok {
				// initial taints upstreamOk regardless of equality with the
				// final status — a 401 first hop is failure even if the
				// chain settled to 200. But only surface the field when it
				// actually differs from the final status; otherwise it's
				// noise.
				if initial < 200 || initial >= 400 {
					ok2xx3xx = false
				}
				if initial != status {
					data["upstreamStatusInitial"] = initial
				}
			}
			data["upstreamOk"] = ok2xx3xx
		}
	}

	return &Response{OK: true, Data: data}, nil
}

// mapStatusToCLIError maps a non-2xx response to a typed *output.CLIError.
// 401 → auth, 403 → forbidden, 404 → not_found, 409 → conflict,
// 429 → rate_limit, 5xx → server, 4xx (other) → usage. The API's error
// message is lifted into Message; the error code is also inspected so
// some 4xx classes re-route — auth-related codes (ApiKeyNotFound,
// ApiKeyInvalid, ...) become ErrAuth, and option-validation codes
// (InvalidOptions, InvalidInput, ...) become ErrValidation. This mirrors
// the v0.9.0 schema-as-docs contract: the API is the validator, and the
// CLI surfaces its responses with consistent error codes.
func mapStatusToCLIError(resp *http.Response, body []byte) *output.CLIError {
	apiMsg, apiCode := extractAPIError(body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || isAuthErrorCode(apiCode):
		msg := apiMsg
		if msg == "" {
			msg = "API rejected the request: not authenticated"
		}
		return output.NewCLIError(output.ErrAuth, msg,
			"Run `urlbox auth --api-secret <secret>` to set or update your API secret.")
	case resp.StatusCode == http.StatusForbidden:
		msg := apiMsg
		if msg == "" {
			msg = "API rejected the request: forbidden"
		}
		return output.NewCLIError(output.ErrForbidden, msg,
			"Your account may not have access to this feature. Check the dashboard.")
	case resp.StatusCode == http.StatusNotFound:
		msg := apiMsg
		if msg == "" {
			msg = "Not found"
		}
		return output.NewCLIError(output.ErrNotFound, msg,
			"Verify the render ID; check the Urlbox dashboard for active renders.")
	case resp.StatusCode == http.StatusConflict:
		msg := apiMsg
		if msg == "" {
			msg = "Conflict"
		}
		return output.NewCLIError(output.ErrConflict, msg,
			"Wait for the in-flight request to complete, or check the dashboard.")
	case resp.StatusCode == http.StatusTooManyRequests:
		msg := apiMsg
		if msg == "" {
			msg = "Rate-limited by the Urlbox API"
		}
		return output.NewCLIError(output.ErrRateLimit, msg,
			"Retry after the cooldown indicated in Retry-After. Consider increasing --max-retries.")
	case resp.StatusCode >= 500:
		msg := apiMsg
		if msg == "" {
			msg = fmt.Sprintf("Urlbox API returned HTTP %d", resp.StatusCode)
		}
		return output.NewCLIError(output.ErrServer, msg,
			"Try again in a moment, or check status.urlbox.com.")
	case isValidationErrorCode(apiCode):
		// v0.9.0 schema-as-docs: --json passes through to the API, so
		// option-validation rejections come back here. Surface them as
		// ErrValidation so the agent's handling matches what would
		// happen for local validation errors.
		msg := apiMsg
		if msg == "" {
			msg = "API rejected one or more options"
		}
		return output.NewCLIError(output.ErrValidation, msg,
			"Run `urlbox render --dry-run` to inspect the merged payload, or `urlbox schema render` to see documented option types and enum values.")
	default: // other 4xx
		msg := apiMsg
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d from Urlbox API", resp.StatusCode)
		}
		return output.NewCLIError(
			output.ErrUsage,
			msg,
			"Inspect the API response in the envelope. Run `urlbox doctor` to verify auth + connectivity, or `urlbox schema render` to confirm option shapes. If the status code looks unmapped, please open an issue with the response code + body.",
		)
	}
}

// isAuthErrorCode returns true when the API error code names an auth
// failure. Urlbox's HTTP status for these is sometimes 400 (e.g.
// ApiKeyNotFound) rather than 401, so we inspect the body code too.
func isAuthErrorCode(code string) bool {
	if code == "" {
		return false
	}
	return strings.HasPrefix(code, "ApiKey") ||
		strings.HasPrefix(code, "Auth") ||
		strings.HasPrefix(code, "Unauthorized")
}

// isValidationErrorCode returns true when the API error code names an
// option-validation failure. These come back as 4xx but should map to
// ErrValidation (not the generic ErrUsage default) so the v0.9.0 schema-as-docs
// contract is honored: --json passes through, and when the API rejects, the
// CLI surfaces a validation envelope. Mirrors the ClientError subclasses in
// urlbox-mono apps/api/src/lib/errors.ts (ValidateRequestErrors namespace
// + a few peers used at the same layer).
func isValidationErrorCode(code string) bool {
	switch code {
	case "InvalidOptions",
		"InvalidInput",
		"InvalidTtl",
		"InvalidQuery",
		"DuplicateThumbnailSuffixError",
		"ConflictingStorageOptionsError":
		return true
	}
	return false
}

// extractAPIError reads an Urlbox-shaped JSON error body and returns
// (message, code). Both shapes are supported:
//
//   - flat:   {"error": "string message"}
//   - nested: {"error": {"code": "ApiKeyNotFound", "message": "..."}}
//   - flat:   {"message": "string message"}
//
// Non-JSON bodies fall back to (trimmed body string, "").
func extractAPIError(body []byte) (msg, code string) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		// Urlbox-specific nested shape: {"error": {"code": "...", "message": "..."}}.
		if errObj, ok := parsed["error"].(map[string]any); ok {
			if c, ok := errObj["code"].(string); ok {
				code = c
			}
			if m, ok := errObj["message"].(string); ok && m != "" {
				return m, code
			}
		}
		// Flat-string shapes.
		for _, k := range []string{"error", "message"} {
			if v, ok := parsed[k].(string); ok && v != "" {
				return v, code
			}
		}
	}
	return trimmed, code
}
