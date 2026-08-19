package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestSessionClientSendsBearerTokenAndUserAgent(t *testing.T) {
	srv := apitest.New(apitest.SuccessJSON(`{"ok":true}`))
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "sess_tok_123")
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/v1/auth/get-session", &out); err != nil {
		t.Fatalf("get: %v", err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer sess_tok_123" {
		t.Fatalf("auth header = %q", got)
	}
	if got := reqs[0].Header.Get("User-Agent"); got == "" {
		t.Fatal("missing User-Agent")
	}
	if reqs[0].Path != "/v1/auth/get-session" {
		t.Fatalf("path = %q", reqs[0].Path)
	}
}

func Test401MapsToAuthWithLoginHint(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{Status: 401, Body: `{"error":{"message":"unauthorized"}}`})
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "sess_expired")
	err := c.GetJSON(context.Background(), "/v2/usage", nil)
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("want CLIError, got %T %v", err, err)
	}
	if cli.Code != output.ErrAuth {
		t.Fatalf("code = %q, want auth", cli.Code)
	}
	if cli.Hint == "" || cli.Hint != "Run `urlbox login` to sign in." {
		t.Fatalf("hint = %q", cli.Hint)
	}
}

func TestDoRawReturnsBodyWithoutMapping(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{Status: 400, Body: `{"error":"authorization_pending"}`})
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "")
	status, data, err := c.DoRaw(context.Background(), "POST", "/v1/auth/device/token", map[string]string{"a": "b"})
	if err != nil {
		t.Fatalf("DoRaw transport error: %v", err)
	}
	if status != 400 {
		t.Fatalf("status = %d", status)
	}
	if data["error"] != "authorization_pending" {
		t.Fatalf("data = %#v", data)
	}
}

func TestSessionClientPutJSONSendsBearerAndBody(t *testing.T) {
	srv := apitest.New(apitest.SuccessJSON(`{"ok":true}`))
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "sess_tok")
	var out map[string]any
	if err := c.PutJSON(context.Background(), "/v2/organisation/org_1/projects/proj_1/proxy", map[string]string{"proxyId": "pool_1"}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("want 1 request, got %d", len(reqs))
	}
	req := reqs[0]
	if req.Method != "PUT" || req.Path != "/v2/organisation/org_1/projects/proj_1/proxy" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sess_tok" {
		t.Fatalf("auth header = %q", got)
	}
	if !strings.Contains(string(req.Body), `"proxyId":"pool_1"`) {
		t.Fatalf("body missing proxyId: %s", req.Body)
	}
}

func TestSessionClientRetries429(t *testing.T) {
	srv := apitest.New(apitest.RetryAfterSeconds(0), apitest.SuccessJSON(`{"fine":true}`))
	t.Cleanup(srv.Close)
	c := NewSessionClient(srv.URL(), "tok")
	var out map[string]any
	if err := c.GetJSON(context.Background(), "/v2/projects", &out); err != nil {
		t.Fatalf("expected retry to succeed: %v", err)
	}
	if len(srv.Requests()) != 2 {
		t.Fatalf("want 2 requests (retry), got %d", len(srv.Requests()))
	}
}
