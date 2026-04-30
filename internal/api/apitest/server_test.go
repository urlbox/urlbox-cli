package apitest_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

// httpGet is a tiny helper that uses NewRequestWithContext (noctx-clean).
func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func TestServer_RecordsRequests(t *testing.T) {
	m := apitest.New(
		apitest.SuccessJSON(`{"ok":true}`),
	)
	t.Cleanup(m.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, m.URL()+"/v1/screenshot", strings.NewReader(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer sec_test")
	req.Header.Set("User-Agent", "urlbox-cli/test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Errorf("body=%q", body)
	}

	reqs := m.Requests()
	if len(reqs) != 1 {
		t.Fatalf("len(reqs)=%d, want 1", len(reqs))
	}
	if reqs[0].Method != "POST" {
		t.Errorf("Method=%q, want POST", reqs[0].Method)
	}
	if reqs[0].Path != "/v1/screenshot" {
		t.Errorf("Path=%q, want /v1/screenshot", reqs[0].Path)
	}
	if reqs[0].Header.Get("Authorization") != "Bearer sec_test" {
		t.Errorf("Authorization=%q", reqs[0].Header.Get("Authorization"))
	}
	if string(reqs[0].Body) != `{"url":"https://example.com"}` {
		t.Errorf("body=%q", reqs[0].Body)
	}
}

func TestServer_RetryAfterSeconds_SetsHeader(t *testing.T) {
	m := apitest.New(
		apitest.RetryAfterSeconds(3),
		apitest.SuccessJSON(`{"ok":true}`),
	)
	t.Cleanup(m.Close)

	resp1 := httpGet(t, m.URL()+"/x")
	defer func() { _ = resp1.Body.Close() }()
	if resp1.StatusCode != 429 {
		t.Errorf("first status=%d, want 429", resp1.StatusCode)
	}
	if resp1.Header.Get("Retry-After") != "3" {
		t.Errorf("Retry-After=%q, want 3", resp1.Header.Get("Retry-After"))
	}

	resp2 := httpGet(t, m.URL()+"/x")
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != 200 {
		t.Errorf("second status=%d, want 200", resp2.StatusCode)
	}
}

func TestServer_ScriptExhaustion_Errors500(t *testing.T) {
	m := apitest.New(apitest.SuccessJSON(`{"ok":true}`))
	t.Cleanup(m.Close)

	resp1 := httpGet(t, m.URL()+"/x")
	_ = resp1.Body.Close()

	resp2 := httpGet(t, m.URL()+"/x")
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != 500 {
		t.Errorf("overflow status=%d, want 500 (script exhausted)", resp2.StatusCode)
	}
}

func TestNeverHitServer_FailsOnRequest(t *testing.T) {
	fakeT := &fakeTesting{}
	s := apitest.NeverHitServer(fakeT)
	t.Cleanup(s.Close)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, s.URL+"/oops", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	if !fakeT.failed {
		t.Errorf("NeverHitServer did not fail the test on hit")
	}
}

func TestServerError_ReturnsStatus(t *testing.T) {
	m := apitest.New(apitest.ServerError(503))
	t.Cleanup(m.Close)

	resp := httpGet(t, m.URL()+"/x")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 503 {
		t.Errorf("status=%d, want 503", resp.StatusCode)
	}
}

type fakeTesting struct {
	failed bool
}

func (f *fakeTesting) Helper()               {}
func (f *fakeTesting) Fatalf(string, ...any) { f.failed = true }
