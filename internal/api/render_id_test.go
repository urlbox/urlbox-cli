package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestRender_SyncSuccess_CapturesRenderIDHeader(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusOK,
		Header: http.Header{
			"Content-Type":        []string{"application/json"},
			"X-Urlbox-Request-Id": []string{"01a0906a-fff5-7565-8dc5-02868f11a4fa_ps"},
		},
		Body: `{"renderUrl":"https://renders.urlbox.com/x.png","size":17752}`,
	})
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.RenderID != "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps" {
		t.Fatalf("RenderID=%q", resp.RenderID)
	}
	if _, ok := resp.Data["renderId"]; ok {
		t.Fatalf("renderId must NOT be injected into Data: %v", resp.Data)
	}
}

func TestRender_SyncSuccess_NoHeader_RenderIDEmpty(t *testing.T) {
	srv := apitest.New(apitest.SuccessJSON(`{"renderUrl":"https://renders.urlbox.com/x.png","size":1}`))
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	resp, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if resp.RenderID != "" {
		t.Fatalf("RenderID=%q, want empty", resp.RenderID)
	}
}

func TestRender_Failure_WrapsRenderIDError(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}`,
	})
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	c.Retry = api.NoRetryConfig()
	_, err := c.Render(context.Background(), map[string]any{"url": "https://x.invalid"})
	var ridErr *api.RenderIDError
	if !errors.As(err, &ridErr) {
		t.Fatalf("want RenderIDError, got %T: %v", err, err)
	}
	if ridErr.RenderID != "01a09082-25cf-74bd-9188-022c4f5bf25f_ps" {
		t.Fatalf("RenderID=%q", ridErr.RenderID)
	}
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("errors.As must still reach the CLIError through Unwrap")
	}
	if cliErr.Code != output.ErrValidation {
		t.Fatalf("code=%q, want validation (InvalidURLError)", cliErr.Code)
	}
}

func TestRender_Failure_NoRequestID_PlainCLIError(t *testing.T) {
	srv := apitest.New(apitest.ScriptedResponse{
		Status: http.StatusUnauthorized,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid token","code":"ApiKeyInvalid"}}`,
	})
	t.Cleanup(srv.Close)
	c := api.NewHTTPClient(srv.URL(), "pk", "sk")
	c.Retry = api.NoRetryConfig()
	_, err := c.Render(context.Background(), map[string]any{"url": "https://example.com"})
	var ridErr *api.RenderIDError
	if errors.As(err, &ridErr) {
		t.Fatalf("no requestId in body must yield a plain CLIError, got RenderIDError")
	}
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != output.ErrAuth {
		t.Fatalf("want auth CLIError, got %v", err)
	}
}
