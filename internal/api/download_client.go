// internal/api/download_client.go — v1.0.4 Class 2.1.
//
// Hardened HTTP client for binary render-output fetches. Separate from
// the JSON-API client (NewHTTPClient) because:
//
//   - Per-request budget is different: PDF/video downloads over slow
//     links can run minutes, where the JSON API has a 30s ceiling.
//   - Redirect policy is stricter here: the render URL is served by
//     Urlbox-controlled HTTPS storage in production, so an HTTPS→HTTP
//     downgrade or a non-http(s) scheme in the chain is a strong signal
//     something hostile is happening. The JSON-API client has no
//     redirects to worry about (a single POST/GET to a known endpoint).
//   - Body-size cap: a misconfigured or malicious renderUrl returning a
//     50 GB body would fill the user's disk before the 5-minute timeout
//     fires. DownloadMaxBytes documents the per-request limit; callers
//     apply it via io.LimitReader at the io.Copy boundary.
//
// Pre-v1.0.4, downloadTo used http.DefaultClient — no MinVersion pin,
// followed up to 10 redirects with no scheme/downgrade rules, no body
// cap. This file closes those three holes.
package api

import (
	"crypto/tls"
	"errors"
	"net/http"
	"time"
)

// DownloadMaxBytes caps the response body size for render downloads.
// 200 MiB covers any realistic screenshot/PDF/video output while
// preventing a malicious or misconfigured renderUrl from filling disk.
// Applied via io.LimitReader at the caller (downloadTo in render_output.go).
const DownloadMaxBytes int64 = 200 * 1024 * 1024

// DownloadTimeout caps a single download attempt. Render outputs are
// usually <10 MiB but over slow corp links a multi-MiB PDF can take
// longer than the JSON-API's 30s budget. 5 minutes mirrors the
// pre-v1.0.4 downloadMaxDuration constant.
const DownloadTimeout = 5 * time.Minute

// maxRedirectHops bounds the redirect chain. Go's default is 10; we
// match it explicitly so the rule survives a future stdlib change.
const maxRedirectHops = 10

// NewDownloadClient returns a hardened *http.Client for fetching render
// bytes from the URL returned by the API. The transport pins TLS >= 1.2;
// CheckRedirect refuses non-http(s) schemes and HTTPS→HTTP downgrades,
// and caps the chain at 10 hops.
func NewDownloadClient() *http.Client {
	return &http.Client{
		Timeout: DownloadTimeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		CheckRedirect: checkDownloadRedirect,
	}
}

// checkDownloadRedirect is the redirect policy for NewDownloadClient.
// Rules:
//
//   - Refuse a redirect target whose scheme is not http or https.
//   - Refuse an HTTPS→HTTP downgrade (the originating request was
//     https, and the next hop is plain http). Plain http→http is
//     accepted because the caller may legitimately be talking to a
//     loopback dev server in tests.
//   - Refuse chains longer than maxRedirectHops.
func checkDownloadRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return errors.New(`refused redirect: scheme must be http or https, got "` + req.URL.Scheme + `"`)
	}
	if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme == "http" {
		return errors.New("refused redirect: HTTPS->HTTP downgrade to " + req.URL.Host)
	}
	if len(via) >= maxRedirectHops {
		return errors.New("refused redirect: too many hops (>" + itoa(maxRedirectHops) + ")")
	}
	return nil
}

// itoa is a tiny helper to avoid importing strconv just for one digit.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
