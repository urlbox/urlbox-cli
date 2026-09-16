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
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
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

func TestRenderHint_Failure_TextTTY_HintUnderError(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}`,
	}
	stdout, stderr, code := runRenderForHint(t, fail,
		"render", "https://x.invalid", "--output-format", "text", "--no-retry")
	if code != 2 {
		t.Fatalf("exit %d, want 2 (validation)\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "urlbox report 01a09082-25cf-74bd-9188-022c4f5bf25f_ps") {
		t.Fatalf("failure hint missing from stderr: %q", stderr)
	}
}

func TestRenderHint_Failure_JSONMode_NoHintInEnvelope(t *testing.T) {
	SetStderrTTYForTest(true)
	t.Cleanup(ResetStderrTTYForTest)
	fail := apitest.ScriptedResponse{
		Status: http.StatusBadRequest,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":{"message":"Invalid URL","code":"InvalidURLError"},"requestId":"01a09082-25cf-74bd-9188-022c4f5bf25f_ps"}`,
	}
	stdout, _, code := runRenderForHint(t, fail,
		"render", "https://x.invalid", "--output-format", "json", "--no-retry")
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if strings.Contains(stdout, "urlbox report") {
		t.Fatalf("json error envelope must not carry the report hint: %q", stdout)
	}
}
