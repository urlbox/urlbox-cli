// internal/cmd/doctor_internal_test.go
package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDoctor_HttpTimeout_AtLeast10s pins Round 5 CI-1: the per-check
// HTTP timeout used to be 5s, which false-failed on cold-container
// invocations where DNS+TCP+TLS to api.urlbox.com exceeded the budget
// even though the warm-cache request returned in ~350ms. 10s is the
// floor — any value below this regresses CI health-check usage.
func TestDoctor_HttpTimeout_AtLeast10s(t *testing.T) {
	const minTimeout = 10 * time.Second
	if httpTimeout < minTimeout {
		t.Errorf("httpTimeout = %v, want >= %v for cold-start DNS resilience (Round 5 CI-1)", httpTimeout, minTimeout)
	}
}

// TestCheckAPIReachable_SendsCLIUserAgent pins that the reachability
// probe identifies itself like every other CLI request. Pre-fix it was
// the only doctor HTTP check going out as Go's default UA
// (Go-http-client), making it unattributable in API logs.
func TestCheckAPIReachable_SendsCLIUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	c := checkAPIReachable(context.Background(), srv.URL)
	if c.Status != "ok" {
		t.Fatalf("check status=%s msg=%s", c.Status, c.Message)
	}
	if !strings.HasPrefix(gotUA, "urlbox-cli/") {
		t.Errorf("User-Agent = %q, want urlbox-cli/… prefix", gotUA)
	}
}
