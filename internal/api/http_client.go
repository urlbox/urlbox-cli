package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// Endpoint paths locked from urlbox-mono spec (apps/api/src/modules/render/render.routes.ts).
const (
	pathSync   = "/v1/screenshot"
	pathAsync  = "/v1/screenshot/async"
	pathStatus = "/v1/render/" // append the URL-escaped renderID
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
	return c.do(ctx, http.MethodPost, pathSync, opts)
}

// RenderAsync queues a render. POST /v1/screenshot/async. Returns 201 with
// {status, renderId, statusUrl}.
func (c *HTTPClient) RenderAsync(ctx context.Context, opts map[string]any) (*Response, error) {
	return c.do(ctx, http.MethodPost, pathAsync, opts)
}

// Status returns the latest state of an async render. GET /v1/render/<renderID>.
func (c *HTTPClient) Status(ctx context.Context, renderID string) (*Response, error) {
	return c.do(ctx, http.MethodGet, pathStatus+url.PathEscape(renderID), nil)
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
		return nil, output.NewCLIError(
			output.ErrNetwork,
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
	return &Response{OK: true, Data: data}, nil
}

// mapStatusToCLIError maps a non-2xx response to a typed *output.CLIError.
// 401 → auth, 403 → forbidden, 404 → not_found, 409 → conflict,
// 429 → rate_limit, 5xx → server, 4xx (other) → usage with the API's
// error message lifted into Message.
func mapStatusToCLIError(resp *http.Response, body []byte) *output.CLIError {
	apiMsg := extractAPIErrorMessage(body)
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
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
		return output.NewCLIError(output.ErrConflict, msg, apiMsg)
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
	default: // other 4xx
		msg := apiMsg
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d from Urlbox API", resp.StatusCode)
		}
		return output.NewCLIError(output.ErrUsage, msg, "")
	}
}

// extractAPIErrorMessage tries to read an `error` or `message` field from a
// JSON error body. Falls back to the trimmed body string if it isn't JSON
// (some upstream errors return plain text).
func extractAPIErrorMessage(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err == nil {
		for _, k := range []string{"error", "message"} {
			if v, ok := parsed[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return trimmed
}
