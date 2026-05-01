package api_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// Compile-time assertion that HTTPClient satisfies Client.
var _ api.Client = (*api.HTTPClient)(nil)

// newTestClient builds an HTTPClient pointed at the mock server with retries
// disabled by default. Individual tests can override .Retry to opt in.
func newTestClient(t *testing.T, m *apitest.Server) *api.HTTPClient {
	t.Helper()
	c := api.NewHTTPClient(m.URL(), "pub_test", "sec_test")
	c.Retry = api.NoRetryConfig()
	return c
}

func TestHTTPClient_Render_Success_PinsRequest(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{
		"renderId": "ps_abc",
		"renderUrl": "https://cdn2.urlbox.io/x.png",
		"size": 245632
	}`))
	t.Cleanup(m.Close)

	c := newTestClient(t, m)
	resp, err := c.Render(context.Background(), map[string]any{
		"url":    "https://example.com",
		"format": "png",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !resp.OK {
		t.Errorf("OK=false")
	}
	if resp.Data["renderId"] != "ps_abc" {
		t.Errorf("renderId=%v", resp.Data["renderId"])
	}

	reqs := m.Requests()
	if len(reqs) != 1 {
		t.Fatalf("len(reqs)=%d", len(reqs))
	}
	r := reqs[0]
	if r.Method != http.MethodPost {
		t.Errorf("Method=%q, want POST", r.Method)
	}
	if r.Path != "/v1/screenshot" {
		t.Errorf("Path=%q, want /v1/screenshot (locked sync endpoint)", r.Path)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer sec_test" {
		t.Errorf("Authorization=%q, want %q", got, "Bearer sec_test")
	}
	if got := r.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type=%q", got)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept=%q", got)
	}
	if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "urlbox-cli/") {
		t.Errorf("User-Agent=%q, want urlbox-cli/...", got)
	}
	if !strings.Contains(string(r.Body), `"url":"https://example.com"`) {
		t.Errorf("body=%q", r.Body)
	}
}

func TestHTTPClient_RenderAsync_PinsRequest(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusCreated,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: `{
			"status": "created",
			"renderId": "ps_abc",
			"statusUrl": "https://api.urlbox.com/v1/render/ps_abc"
		}`,
	})
	t.Cleanup(m.Close)

	c := newTestClient(t, m)
	resp, err := c.RenderAsync(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("RenderAsync: %v", err)
	}
	if resp.Data["renderId"] != "ps_abc" {
		t.Errorf("renderId=%v", resp.Data["renderId"])
	}
	if resp.Data["status"] != "created" {
		t.Errorf("status=%v", resp.Data["status"])
	}
	reqs := m.Requests()
	if reqs[0].Path != "/v1/screenshot/async" {
		t.Errorf("Path=%q, want /v1/screenshot/async", reqs[0].Path)
	}
}

func TestHTTPClient_Status_GetByID(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{
		"renderId": "ps_abc",
		"status": "succeeded",
		"renderUrl": "https://cdn2.urlbox.io/x.png"
	}`))
	t.Cleanup(m.Close)

	c := newTestClient(t, m)
	resp, err := c.Status(context.Background(), "ps_abc")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if resp.Data["status"] != "succeeded" {
		t.Errorf("status=%v", resp.Data["status"])
	}
	reqs := m.Requests()
	if len(reqs) != 1 {
		t.Fatalf("len(reqs)=%d", len(reqs))
	}
	if reqs[0].Method != http.MethodGet {
		t.Errorf("Method=%q, want GET", reqs[0].Method)
	}
	if reqs[0].Path != "/v1/render/ps_abc" {
		t.Errorf("Path=%q, want /v1/render/ps_abc", reqs[0].Path)
	}
	if len(reqs[0].Body) != 0 {
		t.Errorf("Status request should have empty body, got %q", reqs[0].Body)
	}
}

func TestHTTPClient_Status_RenderIDIsURLEscaped(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{"renderId":"weird","status":"succeeded"}`))
	t.Cleanup(m.Close)

	c := newTestClient(t, m)
	_, err := c.Status(context.Background(), "weird/id with space")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	got := m.Requests()[0].RawPath
	if got != "/v1/render/weird%2Fid%20with%20space" {
		t.Errorf("RawPath=%q, want URL-escaped renderID", got)
	}
}

func TestHTTPClient_Render_401_MapsToAuth(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusUnauthorized,
		Body:   `{"error": "invalid api secret"}`,
	})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})

	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("err=%v, want *output.CLIError", err)
	}
	if cli.Code != output.ErrAuth {
		t.Errorf("Code=%q, want %q", cli.Code, output.ErrAuth)
	}
	if !strings.Contains(cli.Hint, "urlbox auth") {
		t.Errorf("Hint=%q, want a pointer to `urlbox auth`", cli.Hint)
	}
}

func TestHTTPClient_Render_403_MapsToForbidden(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{Status: http.StatusForbidden, Body: `{"error":"plan limit"}`})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrForbidden {
		t.Errorf("err=%v, want code=%q", err, output.ErrForbidden)
	}
}

func TestHTTPClient_Render_404_MapsToNotFound(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{Status: http.StatusNotFound, Body: `{"error":"not found"}`})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrNotFound {
		t.Errorf("err=%v, want code=%q", err, output.ErrNotFound)
	}
}

func TestHTTPClient_Render_409_MapsToConflict(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{Status: http.StatusConflict, Body: `{"error":"already pending"}`})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrConflict {
		t.Errorf("err=%v, want code=%q", err, output.ErrConflict)
	}
}

func TestHTTPClient_Render_429_AfterRetryBudget_MapsToRateLimit(t *testing.T) {
	m := apitest.New(
		apitest.RetryAfterSeconds(0),
		apitest.RetryAfterSeconds(0),
		apitest.RetryAfterSeconds(0),
		apitest.RetryAfterSeconds(0),
	)
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = func(time.Duration) {} // no real sleep
	cfg.Jitter = 0
	c.Retry = cfg

	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrRateLimit {
		t.Errorf("err=%v, want code=%q", err, output.ErrRateLimit)
	}
	if got := len(m.Requests()); got != 4 {
		t.Errorf("len(reqs)=%d, want 4 (1+3 retries)", got)
	}
}

func TestHTTPClient_Render_5xx_MapsToServer(t *testing.T) {
	m := apitest.New(apitest.ServerError(http.StatusInternalServerError))
	t.Cleanup(m.Close)
	c := newTestClient(t, m)

	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrServer {
		t.Errorf("err=%v, want code=%q", err, output.ErrServer)
	}
}

func TestHTTPClient_Render_NetworkError_MapsToNetwork(t *testing.T) {
	// Point at a closed port to force a connection-refused error.
	c := api.NewHTTPClient("http://127.0.0.1:1", "pub", "sec")
	c.Retry = api.NoRetryConfig()

	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrNetwork {
		t.Errorf("err=%v, want code=%q", err, output.ErrNetwork)
	}
	if !strings.Contains(cli.Hint, "urlbox doctor") {
		t.Errorf("Hint=%q, want a pointer to `urlbox doctor`", cli.Hint)
	}
}

func TestHTTPClient_Render_4xxOther_MapsToUsage(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{Status: http.StatusBadRequest, Body: `{"error":"missing required field: url"}`})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
		t.Errorf("err=%v, want code=%q", err, output.ErrUsage)
	}
	if !strings.Contains(cli.Message, "missing required field: url") {
		t.Errorf("Message=%q should lift API error", cli.Message)
	}
}

// The real Urlbox API returns 400 (not 401) with a NESTED error body for
// auth failures: {"error": {"code": "ApiKeyNotFound", "message": "..."}}.
// We must still map this to ErrAuth based on the inner code, even though
// the HTTP status code alone would route to ErrUsage.
func TestHTTPClient_Render_400_ApiKeyNotFound_MapsToAuth(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"code":"ApiKeyNotFound","message":"Api Key does not exist"},"requestId":"x"}`,
	})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("err=%v, want *output.CLIError", err)
	}
	if cli.Code != output.ErrAuth {
		t.Errorf("Code=%q, want %q (ApiKey* code on 400 should re-route to auth)", cli.Code, output.ErrAuth)
	}
	if !strings.Contains(cli.Message, "Api Key does not exist") {
		t.Errorf("Message=%q should lift the nested error.message", cli.Message)
	}
	if !strings.Contains(cli.Hint, "urlbox auth") {
		t.Errorf("Hint=%q, want pointer to `urlbox auth`", cli.Hint)
	}
}

func TestHTTPClient_Render_NonJSONErrorBody_FallsBackToBodyString(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{Status: http.StatusBadRequest, Body: `not json at all`})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{})
	var cli *output.CLIError
	if !errors.As(err, &cli) || cli.Code != output.ErrUsage {
		t.Errorf("err=%v, want code=%q", err, output.ErrUsage)
	}
	if !strings.Contains(cli.Message, "not json at all") {
		t.Errorf("Message=%q should fall back to body string", cli.Message)
	}
}
