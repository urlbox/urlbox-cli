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
	if !strings.Contains(cli.Hint, "urlbox login") {
		t.Errorf("Hint=%q, want a pointer to `urlbox login`", cli.Hint)
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
// v0.9.0 schema-as-docs: --json passes through to the API, so option
// rejections come back as 400 with code "InvalidOptions" (or related
// validation codes). These must route to ErrValidation — NOT the
// generic ErrUsage default for "other 4xx" — so agents can match the
// envelope code consistently with what would happen for local validation.
func TestHTTPClient_Render_400_InvalidOptions_MapsToValidation(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"code":"InvalidOptions","message":"Invalid options, please check errors"},"requestId":"x"}`,
	})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com", "width": []any{1, 2, 3}})
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("err=%v, want *output.CLIError", err)
	}
	if cli.Code != output.ErrValidation {
		t.Errorf("Code=%q, want %q (InvalidOptions on 400 should re-route to validation)", cli.Code, output.ErrValidation)
	}
	if !strings.Contains(cli.Message, "Invalid options") {
		t.Errorf("Message=%q should lift the API's rejection message", cli.Message)
	}
	if !strings.Contains(cli.Hint, "schema render") {
		t.Errorf("Hint=%q, want pointer to `urlbox schema render`", cli.Hint)
	}
}

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
	if !strings.Contains(cli.Hint, "urlbox login") {
		t.Errorf("Hint=%q, want pointer to `urlbox login`", cli.Hint)
	}
}

// When the API response includes data.response.statusCode, propagate it
// through Response.Data so the render command can surface it in the
// envelope. The Urlbox API nests upstream status under "response" (see
// urlbox-mono apps/api/src/lib/utils.ts:86-122).
func TestHTTPClient_Render_UpstreamStatus_Propagates(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 245632,
		"renderTime": 1234,
		"width": 1920,
		"height": 1080,
		"response": {"statusCode": 401}
	}`))
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.Data["upstreamOk"] != false {
		t.Errorf("upstreamOk=%v, want false (page returned 401)", resp.Data["upstreamOk"])
	}
	if status, _ := resp.Data["upstreamStatus"].(float64); status != 401 {
		t.Errorf("upstreamStatus=%v, want 401", resp.Data["upstreamStatus"])
	}
}

func TestHTTPClient_Render_UpstreamStatusOK_DefaultsTrue(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 245632,
		"response": {"statusCode": 200}
	}`))
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.Data["upstreamOk"] != true {
		t.Errorf("upstreamOk=%v, want true (page returned 200)", resp.Data["upstreamOk"])
	}
}

// statusCodeInitial captures the first request's code before any redirects.
// A 401-then-302-then-200 login-wall chain must read as upstreamOk=false
// because the initial request failed even if the final landing was 200.
func TestHTTPClient_Render_UpstreamStatusInitial_TaintsOk(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 245632,
		"response": {"statusCode": 200, "statusCodeInitial": 401}
	}`))
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.Data["upstreamOk"] != false {
		t.Errorf("upstreamOk=%v, want false (initial 401 redirected to 200)", resp.Data["upstreamOk"])
	}
	if status, _ := resp.Data["upstreamStatus"].(float64); status != 200 {
		t.Errorf("upstreamStatus=%v, want 200 (final code)", resp.Data["upstreamStatus"])
	}
	if initial, _ := resp.Data["upstreamStatusInitial"].(float64); initial != 401 {
		t.Errorf("upstreamStatusInitial=%v, want 401", resp.Data["upstreamStatusInitial"])
	}
}

// When statusCodeInitial equals the final statusCode, upstreamStatusInitial
// is noise — omit it. The taint logic still runs (still set via initial), but
// agents shouldn't see a duplicate field for the common 200/200 chain.
func TestHTTPClient_Render_UpstreamStatusInitial_OmittedWhenEqual(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 245632,
		"response": {"statusCode": 200, "statusCodeInitial": 200}
	}`))
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.Data["upstreamOk"] != true {
		t.Errorf("upstreamOk=%v, want true (both 200)", resp.Data["upstreamOk"])
	}
	if _, ok := resp.Data["upstreamStatusInitial"]; ok {
		t.Errorf("upstreamStatusInitial should be absent when initial == final (got %v)", resp.Data["upstreamStatusInitial"])
	}
}

func TestHTTPClient_Render_NoResponseObject_OmitsField(t *testing.T) {
	// When the API doesn't include data.response (engine error, empty
	// render), we don't lie — both upstreamOk and upstreamStatus are
	// absent from Data.
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 245632
	}`))
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, ok := resp.Data["upstreamOk"]; ok {
		t.Errorf("upstreamOk should be absent when the API doesn't expose response.statusCode")
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

// TestHTTPClient_Render_InvalidURLError_MapsToValidation pins Round 5
// First-2: when the API returns HTTP 400 with apiCode="InvalidURLError"
// (typical for unreachable target URLs like https://nonexistent.invalid),
// the CLI used to map it to ErrUsage (exit 1) — implying the user
// misused the CLI. But the user passed a syntactically-valid URL; the
// failure is that the API couldn't reach the target. ErrValidation
// (exit 2) is the more accurate class: "your input was rejected".
func TestHTTPClient_Render_InvalidURLError_MapsToValidation(t *testing.T) {
	m := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"}}`,
	})
	t.Cleanup(m.Close)
	c := newTestClient(t, m)
	_, err := c.Render(context.Background(), map[string]any{"url": "https://nonexistent.invalid"})

	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("err=%v, want *output.CLIError", err)
	}
	if cli.Code != output.ErrValidation {
		t.Errorf("Code=%q, want %q (Round 5 First-2)", cli.Code, output.ErrValidation)
	}
	if !strings.Contains(cli.Message, "Invalid URL") {
		t.Errorf("Message=%q should surface the API's text", cli.Message)
	}
}
