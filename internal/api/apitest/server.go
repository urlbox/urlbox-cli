// Package apitest provides reusable httptest scaffolding for testing the
// internal/api Client. Tests in internal/api/, internal/cmd/render*, and
// elsewhere import this package to avoid touching real api.urlbox.com.
//
// This is a separate package (not a `_test.go` helper file) so its types
// can be imported across multiple test packages while staying out of the
// production binary.
package apitest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
)

// Server is a thin wrapper around httptest.Server that records every
// inbound request and returns scripted responses in order.
type Server struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []CapturedRequest
	scripts  []ScriptedResponse
	cursor   int
}

// CapturedRequest records a single inbound HTTP request for later assertion.
// Body is read fully and stored as a defensive copy.
type CapturedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// ScriptedResponse is a one-shot response written by Server. The mock pops
// one off the front per request; if it runs out, it 500s with a clear body.
type ScriptedResponse struct {
	Status int
	Header http.Header // optional (Retry-After, Content-Type, ...)
	Body   string      // raw body; if empty, no body is written
}

// TestingT is the subset of testing.TB used by NeverHitServer. Defined
// locally so this package doesn't import testing at file scope.
type TestingT interface {
	Helper()
	Fatalf(format string, args ...any)
}

// New returns a Server pre-loaded with the given scripted responses.
// Caller must Close() (typically via t.Cleanup).
func New(scripts ...ScriptedResponse) *Server {
	m := &Server{scripts: scripts}
	m.server = httptest.NewServer(http.HandlerFunc(m.serve))
	return m
}

// URL returns the server's base URL (use as APIHost in HTTPClient under test).
func (m *Server) URL() string { return m.server.URL }

// Close shuts down the underlying httptest server.
func (m *Server) Close() { m.server.Close() }

// Requests returns a defensive copy of every captured request.
func (m *Server) Requests() []CapturedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]CapturedRequest, len(m.requests))
	copy(out, m.requests)
	return out
}

func (m *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	m.mu.Lock()
	m.requests = append(m.requests, CapturedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
		Body:   body,
	})
	if m.cursor >= len(m.scripts) {
		m.mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("apitest: scripts exhausted"))
		return
	}
	resp := m.scripts[m.cursor]
	m.cursor++
	m.mu.Unlock()

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if resp.Status != 0 {
		w.WriteHeader(resp.Status)
	}
	if resp.Body != "" {
		_, _ = w.Write([]byte(resp.Body))
	}
}

// NeverHitServer returns an httptest.Server whose handler immediately fails
// the test on any request. Use for --dry-run / --curl tests that MUST NOT
// touch the network.
func NeverHitServer(t TestingT) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Helper()
		t.Fatalf("dry-run / curl test made an HTTP call (path=%s)", r.URL.Path)
	}))
}

// SuccessJSON is sugar for the most common case: a 200 response with a JSON body.
func SuccessJSON(body string) ScriptedResponse {
	return ScriptedResponse{
		Status: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   body,
	}
}

// RetryAfterSeconds returns a 429 response with the given Retry-After value
// (in seconds). Body is empty.
func RetryAfterSeconds(n int) ScriptedResponse {
	return ScriptedResponse{
		Status: http.StatusTooManyRequests,
		Header: http.Header{"Retry-After": []string{strconv.Itoa(n)}},
	}
}

// ServerError returns a response with the given 5xx status and an empty body.
func ServerError(status int) ScriptedResponse {
	return ScriptedResponse{Status: status}
}
