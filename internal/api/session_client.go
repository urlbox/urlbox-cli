package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// SessionAPI is the session-authenticated request surface every
// account-management command depends on, so commands and their tests can
// target this interface rather than the concrete client.
type SessionAPI interface {
	GetJSON(ctx context.Context, path string, out any) error
	PostJSON(ctx context.Context, path string, body, out any) error
	PatchJSON(ctx context.Context, path string, body, out any) error
	PutJSON(ctx context.Context, path string, body, out any) error
	DeleteJSON(ctx context.Context, path string, out any) error
}

// SessionClient talks to the Urlbox API using a session bearer token rather
// than an API secret. It rides the shared request path (RetryDo, error
// mapping, user-agent); the only differences are the credential and the 401
// mapping (auth + "Run `urlbox login`" hint).
type SessionClient struct {
	baseURL   string
	token     string
	userAgent string
	timeout   time.Duration
	retry     RetryConfig
	http      *http.Client
}

var _ SessionAPI = (*SessionClient)(nil)

// NewSessionClient constructs a SessionClient for baseURL authenticated with
// the given session token. baseURL must be non-empty.
func NewSessionClient(baseURL, token string) *SessionClient {
	timeout := 30 * time.Second
	return &SessionClient{
		baseURL:   baseURL,
		token:     token,
		userAgent: BuildUserAgent(version.Version),
		timeout:   timeout,
		retry:     DefaultRetryConfig(),
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
	}
}

// SetRetryConfig overrides the client's retry policy. Used by the central
// session-client builder to thread the --no-retry / --max-retries flags in;
// NewSessionClient keeps DefaultRetryConfig() for callers that don't set one.
func (c *SessionClient) SetRetryConfig(cfg RetryConfig) {
	c.retry = cfg
}

// GetJSON performs a GET and decodes the response body into out (may be nil).
func (c *SessionClient) GetJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

// PostJSON performs a POST with a JSON body and decodes the response into out.
func (c *SessionClient) PostJSON(ctx context.Context, path string, body, out any) error {
	if body == nil {
		body = map[string]string{}
	}
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}

// PatchJSON performs a PATCH with a JSON body and decodes the response into out.
func (c *SessionClient) PatchJSON(ctx context.Context, path string, body, out any) error {
	return c.doJSON(ctx, http.MethodPatch, path, body, out)
}

// PutJSON performs a PUT with a JSON body and decodes the response into out.
func (c *SessionClient) PutJSON(ctx context.Context, path string, body, out any) error {
	return c.doJSON(ctx, http.MethodPut, path, body, out)
}

// DeleteJSON performs a DELETE and decodes the response body into out (may be nil).
func (c *SessionClient) DeleteJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodDelete, path, nil, out)
}

func (c *SessionClient) doJSON(ctx context.Context, method, path string, body, out any) error {
	resp, respBody, err := c.send(ctx, method, path, body) //nolint:bodyclose // send closes the body before returning
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		cli := mapStatusToCLIError(resp, respBody)
		if cli.Code == output.ErrAuth {
			return output.NewCLIError(
				output.ErrAuth,
				cli.Message,
				"Run `urlbox login` to sign in.",
			)
		}
		return cli
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return output.NewCLIError(output.ErrServer, "failed to parse API response", err.Error())
	}
	return nil
}

// DoRaw sends a request and returns the status and parsed JSON body without
// mapping non-2xx responses to CLI errors. The device-poll loop reads RFC
// error strings (e.g. authorization_pending) straight from the raw body.
func (c *SessionClient) DoRaw(ctx context.Context, method, path string, body any) (status int, data map[string]any, err error) {
	resp, respBody, sendErr := c.send(ctx, method, path, body) //nolint:bodyclose // send closes the body before returning
	if sendErr != nil {
		return 0, nil, sendErr
	}
	data = map[string]any{}
	if len(respBody) > 0 {
		if jerr := json.Unmarshal(respBody, &data); jerr != nil {
			return resp.StatusCode, map[string]any{}, nil
		}
	}
	return resp.StatusCode, data, nil
}

func (c *SessionClient) send(ctx context.Context, method, path string, body any) (*http.Response, []byte, error) {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, output.NewCLIError(output.ErrUsage, "failed to encode request body", err.Error())
		}
		bodyBytes = b
	}
	send := func() (*http.Response, error) {
		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
		if err != nil {
			return nil, err
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		if bodyBytes != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		return c.http.Do(req)
	}
	resp, err := RetryDo(ctx, c.retry, send)
	if err != nil {
		code := output.ErrNetwork
		if errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			code = output.ErrTimeout
		}
		return nil, nil, output.NewCLIError(code, err.Error(),
			"Check your internet connection and the API host (URLBOX_API_HOST).")
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, nil, output.NewCLIError(output.ErrNetwork, readErr.Error(), "Check your internet connection.")
	}
	return resp, respBody, nil
}
