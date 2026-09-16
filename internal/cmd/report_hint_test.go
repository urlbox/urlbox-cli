package cmd

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const syncSuccessBody = `{"renderUrl":"https://renders.urlbox.com/x.png","size":17752}`

func syncSuccessWithHeader() apitest.ScriptedResponse {
	return apitest.ScriptedResponse{
		Status: http.StatusOK,
		Header: http.Header{
			"Content-Type":        []string{"application/json"},
			"X-Urlbox-Request-Id": []string{"01a0906a-fff5-7565-8dc5-02868f11a4fa_ps"},
		},
		Body: syncSuccessBody,
	}
}

func runRenderForHint(t *testing.T, script apitest.ScriptedResponse, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	return runRenderForHintWithSession(t, script, true, args...)
}

// runRenderForHintWithSession is runRenderForHint with control over whether
// the profile carries a session token (the report command needs one).
func runRenderForHintWithSession(t *testing.T, script apitest.ScriptedResponse, withSession bool, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	dir := t.TempDir()
	writeCompatConfig(t, dir, withSession)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(script)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestRenderHint_SyncSuccess_TextTTY_PrintsHintToStderr(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "urlbox report 01a0906a-fff5-7565-8dc5-02868f11a4fa_ps") {
		t.Fatalf("stderr missing hint: %q", stderr)
	}
	if strings.Contains(stdout, "urlbox report") {
		t.Fatalf("hint leaked to stdout: %q", stdout)
	}
}

func TestRenderHint_SyncSuccess_NonTTYStderr_NoHint(t *testing.T) {
	SetStderrTTYForTest(false)
	t.Cleanup(ResetStderrTTYForTest)
	_, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("hint must be absent off-TTY: %q", stderr)
	}
}

func TestRenderHint_JSONMode_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "urlbox report") || strings.Contains(stdout, "urlbox report") {
		t.Fatalf("hint must be absent in json mode\nstdout=%q\nstderr=%q", stdout, stderr)
	}
	if strings.Contains(stdout, "renderId") {
		t.Fatalf("sync json envelope must not gain a renderId field: %q", stdout)
	}
}

func TestRenderHint_QuietMode_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHint(t, syncSuccessWithHeader(),
		"render", "https://example.com", "--output-format", "quiet")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stdout+stderr, "urlbox report") {
		t.Fatalf("hint must be absent in quiet mode")
	}
}

func TestRenderHint_SyncSuccess_NoHeader_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	_, stderr, code := runRenderForHint(t, apitest.SuccessJSON(syncSuccessBody),
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("no id captured, no hint: %q", stderr)
	}
}

func TestRenderHint_NoSession_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	stdout, stderr, code := runRenderForHintWithSession(t, syncSuccessWithHeader(), false,
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("secret-only profile cannot file a report, so no hint: %q", stderr)
	}
}

func TestRenderHint_NoSession_ServerFailure_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := failureResponse(http.StatusInternalServerError,
		`{"error":{"message":"Render failed","code":"RenderError"},"requestId":"`+failureRequestID+`"}`)
	_, stderr, code := runRenderForHintWithSession(t, fail, false,
		"render", "https://example.com", "--output-format", "text", "--no-retry")
	if code != 10 {
		t.Fatalf("exit %d, want 10\n%s", code, stderr)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("secret-only profile must not get the failure hint either: %q", stderr)
	}
}

func TestRenderHint_CacheHit_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	hit := syncSuccessWithHeader()
	hit.Header.Set("X-Urlbox-Cache-Status", "HIT")
	stdout, stderr, code := runRenderForHint(t, hit,
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("a cached render has no render record to report, so no hint: %q", stderr)
	}
}

func TestRenderHint_CacheMiss_StillHints(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	miss := syncSuccessWithHeader()
	miss.Header.Set("X-Urlbox-Cache-Status", "MISS")
	_, stderr, code := runRenderForHint(t, miss,
		"render", "https://example.com", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr)
	}
	if !strings.Contains(stderr, "urlbox report 01a0906a-fff5-7565-8dc5-02868f11a4fa_ps") {
		t.Fatalf("cache MISS must keep the hint: %q", stderr)
	}
}

func TestRenderHint_Async_UsesBodyRenderID(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	_, stderr, code := runRenderForHint(t,
		apitest.SuccessJSON(`{"status":"created","renderId":"01a09072-9ec6-76ca-bcc7-bbf5cc35a9e4_pa","statusUrl":"https://api.urlbox.com/v1/render/x"}`),
		"render", "https://example.com", "--async", "--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr, "urlbox report 01a09072-9ec6-76ca-bcc7-bbf5cc35a9e4_pa") {
		t.Fatalf("async hint missing: %q", stderr)
	}
}

const failureRequestID = "01a09082-25cf-74bd-9188-022c4f5bf25f_ps"

func failureResponse(status int, body string) apitest.ScriptedResponse {
	return apitest.ScriptedResponse{
		Status: status,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   body,
	}
}

func TestRenderHint_ServerFailure_TextTTY_HintUnderError(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := failureResponse(http.StatusInternalServerError,
		`{"error":{"message":"Render failed","code":"RenderError"},"requestId":"`+failureRequestID+`"}`)
	stdout, stderr, code := runRenderForHint(t, fail,
		"render", "https://example.com", "--output-format", "text", "--no-retry")
	if code != 10 {
		t.Fatalf("exit %d, want 10 (server)\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "urlbox report "+failureRequestID) {
		t.Fatalf("server failure hint missing from stderr: %q", stderr)
	}
}

func TestRenderHint_ValidationFailure_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := failureResponse(http.StatusBadRequest,
		`{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"`+failureRequestID+`"}`)
	stdout, stderr, code := runRenderForHint(t, fail,
		"render", "https://x.invalid", "--output-format", "text", "--no-retry")
	if code != 2 {
		t.Fatalf("exit %d, want 2 (validation)\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("validation error must not carry the report hint: %q", stderr)
	}
}

func TestRenderHint_AuthFailure_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := failureResponse(http.StatusUnauthorized,
		`{"error":{"message":"Invalid API key","code":"ApiKeyNotFound"},"requestId":"`+failureRequestID+`"}`)
	stdout, stderr, code := runRenderForHint(t, fail,
		"render", "https://example.com", "--output-format", "text", "--no-retry")
	if code != 3 {
		t.Fatalf("exit %d, want 3 (auth)\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("auth error must not carry the report hint: %q", stderr)
	}
}

func TestRenderHint_RateLimitFailure_NoHint(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := failureResponse(http.StatusTooManyRequests,
		`{"error":{"message":"Rate limited","code":"RateLimited"},"requestId":"`+failureRequestID+`"}`)
	stdout, stderr, code := runRenderForHint(t, fail,
		"render", "https://example.com", "--output-format", "text", "--no-retry")
	if code != 6 {
		t.Fatalf("exit %d, want 6 (rate_limit)\n%s\n%s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "urlbox report") {
		t.Fatalf("rate-limit error must not carry the report hint: %q", stderr)
	}
}

func TestRenderHint_Failure_JSONMode_NoHintInEnvelope(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := failureResponse(http.StatusInternalServerError,
		`{"error":{"message":"Render failed","code":"RenderError"},"requestId":"`+failureRequestID+`"}`)
	stdout, _, code := runRenderForHint(t, fail,
		"render", "https://example.com", "--output-format", "json", "--no-retry")
	if code != 10 {
		t.Fatalf("exit %d, want 10", code)
	}
	if strings.Contains(stdout, "urlbox report") {
		t.Fatalf("json error envelope must not carry the report hint: %q", stdout)
	}
}
