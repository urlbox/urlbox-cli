// internal/api/download_client_test.go — v1.0.4 Class 2.1.
//
// Pins the hardened render-download HTTP client contract: TLS 1.2 min,
// no non-http(s) redirects, no HTTPS→HTTP downgrades, no unbounded
// hop chains, and a sane body-size cap exposed for caller-side
// io.LimitReader use.
package api_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
)

func TestNewDownloadClient_TLSMinVersionPinned(t *testing.T) {
	c := api.NewDownloadClient()
	transport, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatalf("transport.TLSClientConfig is nil — download client must pin TLS")
	}
	if transport.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Errorf("download client must pin TLS >= 1.2, got %d", transport.TLSClientConfig.MinVersion)
	}
}

func TestNewDownloadClient_TimeoutSet(t *testing.T) {
	c := api.NewDownloadClient()
	if c.Timeout <= 0 {
		t.Errorf("download client must have a per-request Timeout > 0, got %v", c.Timeout)
	}
}

func TestNewDownloadClient_CheckRedirectExists(t *testing.T) {
	c := api.NewDownloadClient()
	if c.CheckRedirect == nil {
		t.Fatalf("download client must set CheckRedirect to enforce redirect policy")
	}
}

// reqAt builds an *http.Request at the given URL with no body for
// CheckRedirect input. CheckRedirect's contract: req is the about-to-
// be-followed request; via is the chain of preceding requests.
func reqAt(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		t.Fatalf("build req %q: %v", rawURL, err)
	}
	return req
}

func TestNewDownloadClient_Redirect_RefusesNonHTTPScheme(t *testing.T) {
	c := api.NewDownloadClient()
	for _, target := range []string{
		"file:///etc/passwd",
		"javascript:alert(1)",
		"ftp://evil.example/x",
		"chrome://settings",
	} {
		via := []*http.Request{reqAt(t, "https://api.urlbox.com/v1/render/x")}
		err := c.CheckRedirect(reqAt(t, target), via)
		if err == nil {
			t.Errorf("CheckRedirect should refuse %q, got nil", target)
		}
	}
}

func TestNewDownloadClient_Redirect_RefusesHTTPSDowngrade(t *testing.T) {
	c := api.NewDownloadClient()
	via := []*http.Request{reqAt(t, "https://api.urlbox.com/v1/render/x")}
	err := c.CheckRedirect(reqAt(t, "http://attacker.example/x"), via)
	if err == nil {
		t.Fatalf("CheckRedirect must refuse HTTPS→HTTP downgrade")
	}
	if !strings.Contains(err.Error(), "downgrade") && !strings.Contains(err.Error(), "http") {
		t.Errorf("downgrade error should mention http; got %v", err)
	}
}

func TestNewDownloadClient_Redirect_AllowsHTTPSToHTTPS(t *testing.T) {
	c := api.NewDownloadClient()
	via := []*http.Request{reqAt(t, "https://api.urlbox.com/v1/render/x")}
	err := c.CheckRedirect(reqAt(t, "https://cdn.urlbox.com/render-xyz.png"), via)
	if err != nil {
		t.Errorf("HTTPS→HTTPS redirect should succeed; got %v", err)
	}
}

func TestNewDownloadClient_Redirect_AllowsHTTPLoopback(t *testing.T) {
	// Local dev / httptest target may already be plain http://127.0.0.1.
	// If the original request itself was http (loopback), redirects within
	// http are fine — we only refuse downgrades from https.
	c := api.NewDownloadClient()
	via := []*http.Request{reqAt(t, "http://127.0.0.1:8080/render")}
	err := c.CheckRedirect(reqAt(t, "http://127.0.0.1:8080/render-xyz.png"), via)
	if err != nil {
		t.Errorf("HTTP→HTTP loopback redirect should succeed; got %v", err)
	}
}

func TestNewDownloadClient_Redirect_RefusesTooManyHops(t *testing.T) {
	c := api.NewDownloadClient()
	// Build a 10-hop history.
	via := make([]*http.Request, 10)
	for i := range via {
		via[i] = reqAt(t, "https://api.urlbox.com/hop")
	}
	err := c.CheckRedirect(reqAt(t, "https://api.urlbox.com/hop11"), via)
	if err == nil {
		t.Errorf("CheckRedirect must refuse >10 hops")
	}
}

func TestDownloadMaxBytes_Sane(t *testing.T) {
	// 200 MB is generous for a screenshot/PDF and prevents disk-fill DoS.
	// Pin exact value so a careless edit doesn't silently raise the cap.
	want := int64(200 * 1024 * 1024)
	if api.DownloadMaxBytes != want {
		t.Errorf("DownloadMaxBytes = %d, want %d (200 MiB)", api.DownloadMaxBytes, want)
	}
}

func TestDownloadTimeout_MatchesPreV104Behaviour(t *testing.T) {
	// Pre-v1.0.4 downloadMaxDuration was 5 minutes. Keep the budget identical
	// to avoid silently regressing slow-link PDFs.
	if api.DownloadTimeout.Minutes() != 5 {
		t.Errorf("DownloadTimeout = %v, want 5m (matches pre-v1.0.4 downloadMaxDuration)", api.DownloadTimeout)
	}
}
