// internal/cmd/status_test.go
package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// TestStatus_Succeeded_WrapsInEnvelope confirms a 200 + status=succeeded body
// produces an ok envelope (exit 0) with a breadcrumb suggesting the renderUrl
// be opened.
func TestStatus_Succeeded_WrapsInEnvelope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	body := `{
		"status": "succeeded",
		"renderId": "ps_abc123",
		"renderUrl": "https://renders.urlbox.com/v1/renders/ps_abc123.png",
		"size": 245632
	}`
	m := apitest.New(apitest.SuccessJSON(body))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc123",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Errorf("ok != true: %v", env["ok"])
	}
	if env["command"] != "status" {
		t.Errorf("command != 'status': %v", env["command"])
	}
	data, _ := env["data"].(map[string]any)
	if data["status"] != "succeeded" {
		t.Errorf("data.status=%v, want succeeded", data["status"])
	}
	if data["renderUrl"] != "https://renders.urlbox.com/v1/renders/ps_abc123.png" {
		t.Errorf("data.renderUrl=%v", data["renderUrl"])
	}

	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Fatalf("breadcrumbs empty for succeeded status; want open suggestion")
	}
	bc := bcs[0].(map[string]any)
	if !strings.Contains(bc["cmd"].(string), "https://renders.urlbox.com/v1/renders/ps_abc123.png") {
		t.Errorf("breadcrumb cmd should reference renderUrl; got %v", bc["cmd"])
	}
}

// TestStatus_Processing_OK_BreadcrumbToWait confirms an in-flight status
// (created/retrying/processing) yields exit 0 + a breadcrumb suggesting --wait.
func TestStatus_Processing_OK_BreadcrumbToWait(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	body := `{
		"status": "created",
		"renderId": "ps_abc123"
	}`
	m := apitest.New(apitest.SuccessJSON(body))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc123",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["ok"] != true {
		t.Errorf("ok != true (in-flight should still be ok=true): %v", env["ok"])
	}
	data, _ := env["data"].(map[string]any)
	if data["status"] != "created" {
		t.Errorf("data.status=%v, want created", data["status"])
	}
	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Fatalf("breadcrumbs empty; want --wait suggestion")
	}
	bc := bcs[0].(map[string]any)
	cmdStr, _ := bc["cmd"].(string)
	if !strings.Contains(cmdStr, "--wait") {
		t.Errorf("breadcrumb should suggest --wait; got %q", cmdStr)
	}
	if !strings.Contains(cmdStr, "ps_abc123") {
		t.Errorf("breadcrumb should reference renderId; got %q", cmdStr)
	}
}

// TestStatus_Failed_ServerError_Exit10 confirms status=failed returns an
// error envelope with code "server" (exit 10) and surfaces the API's error
// message.
func TestStatus_Failed_ServerError_Exit10(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	body := `{
		"status": "failed",
		"renderId": "ps_abc123",
		"error": "navigation timeout"
	}`
	m := apitest.New(apitest.SuccessJSON(body))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc123",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 10 {
		t.Fatalf("exit=%d, want 10 (server); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != false {
		t.Errorf("ok != false: %v", env["ok"])
	}
	if env["code"] != "server" {
		t.Errorf("code != 'server': %v", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "navigation timeout") {
		t.Errorf("error should include API failure message; got %q", errMsg)
	}
}

// TestStatus_404_NotFound_Exit5 confirms a 404 from the API yields exit 5
// with code "not_found".
func TestStatus_404_NotFound_Exit5(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	notFound := apitest.ScriptedResponse{
		Status: http.StatusNotFound,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"error":"render not found"}`,
	}
	m := apitest.New(notFound)
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_does_not_exist",
		"--no-retry",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 5 {
		t.Fatalf("exit=%d, want 5 (not_found); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != false {
		t.Errorf("ok != false: %v", env["ok"])
	}
	if env["code"] != "not_found" {
		t.Errorf("code != 'not_found': %v", env["code"])
	}
}

// TestStatus_NoArgs_UsageError confirms a missing positional renderId yields
// exit 1 with code "usage".
func TestStatus_NoArgs_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["code"] != "usage" {
		t.Errorf("code != 'usage': %v", env["code"])
	}
}

// scriptedStatusClient is a fake api.Client whose Status() method returns
// successive Responses from the script. Once exhausted it loops on the last
// entry. Used by the --wait tests to drive the polling loop through a known
// transcript.
type scriptedStatusClient struct {
	script []*api.Response
	errs   []error
	calls  atomic.Int32
}

func (s *scriptedStatusClient) Render(_ context.Context, _ map[string]any) (*api.Response, error) {
	return nil, fmt.Errorf("Render not used in --wait tests")
}

func (s *scriptedStatusClient) RenderAsync(_ context.Context, _ map[string]any) (*api.Response, error) {
	return nil, fmt.Errorf("RenderAsync not used in --wait tests")
}

func (s *scriptedStatusClient) Status(_ context.Context, _ string) (*api.Response, error) {
	i := int(s.calls.Add(1)) - 1
	if i >= len(s.script) {
		i = len(s.script) - 1
	}
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	return s.script[i], err
}

// startFakeClockDriver runs a goroutine that wakes any sleeper on fc by
// advancing past the interval. Returns a stop func tests should defer.
//
// Has a sanity cap of 100 advances — well past any well-behaved test's
// budget but tight enough to fail fast if production code stops returning.
func startFakeClockDriver(t *testing.T, fc *clock.FakeClock, advance time.Duration) (stop func()) {
	t.Helper()
	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		const maxAdvances = 100
		advances := 0
		for {
			select {
			case <-stopCh:
				return
			default:
			}
			// Use a short timeout so we re-check stopCh quickly after the
			// production code returns.
			if !fc.WaitForSleeper(50 * time.Millisecond) {
				continue
			}
			fc.Advance(advance)
			advances++
			if advances >= maxAdvances {
				t.Errorf("startFakeClockDriver: hit maxAdvances=%d; production code likely stuck in poll loop", maxAdvances)
				return
			}
		}
	}()
	return func() {
		close(stopCh)
		<-done
	}
}

// TestStatus_Wait_PollsUntilSucceeded confirms --wait polls Status() until it
// returns status=succeeded, then emits the success envelope (exit 0).
// Uses a FakeClock so the synthetic poll-interval (5s) is driven by Advance,
// not real wall-clock waits.
func TestStatus_Wait_PollsUntilSucceeded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	sc := &scriptedStatusClient{
		script: []*api.Response{
			{OK: true, Data: map[string]any{"status": "created", "renderId": "ps_abc"}},
			{OK: true, Data: map[string]any{"status": "processing", "renderId": "ps_abc"}},
			{OK: true, Data: map[string]any{
				"status":    "succeeded",
				"renderId":  "ps_abc",
				"renderUrl": "https://renders.urlbox.com/v1/renders/ps_abc.png",
			}},
		},
	}
	cmd.SetStatusClientForTest(sc)
	t.Cleanup(cmd.ResetStatusClientForTest)

	fc := clock.NewFake(time.Now())
	cmd.SetStatusClockForTest(fc)
	t.Cleanup(cmd.ResetStatusClockForTest)

	stop := startFakeClockDriver(t, fc, 6*time.Second)
	defer stop()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc",
		"--wait",
		"--poll-interval", "5s",
		"--timeout", "30s",
		"--output-format", "json",
	}, &stdout, &stderr)

	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}
	if got := sc.calls.Load(); got != 3 {
		t.Errorf("Status() calls=%d, want 3 (created → processing → succeeded)", got)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Errorf("ok != true: %v", env["ok"])
	}
	data, _ := env["data"].(map[string]any)
	if data["status"] != "succeeded" {
		t.Errorf("data.status=%v, want succeeded", data["status"])
	}
}

// TestStatus_Wait_FailedTerminal_Exit10 confirms a status=failed mid-poll
// short-circuits the loop and yields exit 10 with code "server".
func TestStatus_Wait_FailedTerminal_Exit10(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	sc := &scriptedStatusClient{
		script: []*api.Response{
			{OK: true, Data: map[string]any{"status": "created", "renderId": "ps_abc"}},
			{OK: true, Data: map[string]any{
				"status":   "failed",
				"renderId": "ps_abc",
				"error":    "navigation timeout",
			}},
		},
	}
	cmd.SetStatusClientForTest(sc)
	t.Cleanup(cmd.ResetStatusClientForTest)

	fc := clock.NewFake(time.Now())
	cmd.SetStatusClockForTest(fc)
	t.Cleanup(cmd.ResetStatusClockForTest)

	stop := startFakeClockDriver(t, fc, 6*time.Second)
	defer stop()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc",
		"--wait",
		"--poll-interval", "5s",
		"--timeout", "30s",
		"--output-format", "json",
	}, &stdout, &stderr)

	if exit != 10 {
		t.Fatalf("exit=%d, want 10 (server); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["code"] != "server" {
		t.Errorf("code != 'server': %v", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "navigation timeout") {
		t.Errorf("error should include API failure message; got %q", errMsg)
	}
}

// TestStatus_Wait_TimesOut_TimeoutExit confirms that when --timeout elapses
// before a terminal status is observed, runStatusWait returns ErrTimeout
// (exit 11) with a message naming the renderId, the duration, and the last
// status. Round 1 review (Arch I2 + UX I7): a deadline-exceeded outcome is
// what ErrTimeout exists for; exit 1 (ErrUsage) conflated poll-timeouts with
// "bad flag" usage errors and broke agent retry classification.
func TestStatus_Wait_TimesOut_TimeoutExit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	// Always in-flight — never reaches succeeded/failed.
	sc := &scriptedStatusClient{
		script: []*api.Response{
			{OK: true, Data: map[string]any{"status": "processing", "renderId": "ps_abc"}},
		},
	}
	cmd.SetStatusClientForTest(sc)
	t.Cleanup(cmd.ResetStatusClientForTest)

	fc := clock.NewFake(time.Now())
	cmd.SetStatusClockForTest(fc)
	t.Cleanup(cmd.ResetStatusClockForTest)

	// Drive the clock past the deadline. With timeout=10s and
	// poll-interval=5s, after one poll the next deadline check is at +5s
	// (fits) → sleep → advance to +6s. Second iteration: +6s+5s=+11s,
	// which is past the 10s deadline → returns timeout.
	stop := startFakeClockDriver(t, fc, 6*time.Second)
	defer stop()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc",
		"--wait",
		"--poll-interval", "5s",
		"--timeout", "10s",
		"--output-format", "json",
	}, &stdout, &stderr)

	if exit != 11 {
		t.Fatalf("exit=%d, want 11 (timeout); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("error not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["code"] != "timeout" {
		t.Errorf("code != 'timeout': %v", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "ps_abc") {
		t.Errorf("error should reference renderId; got %q", errMsg)
	}
	if !strings.Contains(strings.ToLower(errMsg), "timed out") {
		t.Errorf("error should say 'timed out'; got %q", errMsg)
	}
	if !strings.Contains(errMsg, "processing") {
		t.Errorf("error should include last status; got %q", errMsg)
	}
}

// TestStatus_Wait_LaterPollContextTimeout_DoesNotSayShorterThanOneCall
// pins Arch I1: the "shorter than a single API call" friendly message
// must only fire on attempt 0. If a per-poll context-deadline fires on
// attempt N>0 (e.g., a slow GET several polls into a long --wait), the
// friendly message lies — by definition the first call already succeeded.
// Fall through to the generic waitTimeoutError instead.
func TestStatus_Wait_LaterPollContextTimeout_DoesNotSayShorterThanOneCall(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	// First call succeeds with "processing"; second call returns a
	// synthetic ErrTimeout. The faux client lets us simulate the
	// per-poll context cancel without needing httptest+sleep timing.
	sc := &scriptedStatusClient{
		script: []*api.Response{
			{OK: true, Data: map[string]any{"status": "processing", "renderId": "ps_late"}},
			nil,
		},
		errs: []error{
			nil,
			output.NewCLIError(output.ErrTimeout, "context deadline exceeded", "Retry the call."),
		},
	}
	cmd.SetStatusClientForTest(sc)
	t.Cleanup(cmd.ResetStatusClientForTest)

	fc := clock.NewFake(time.Now())
	cmd.SetStatusClockForTest(fc)
	t.Cleanup(cmd.ResetStatusClockForTest)
	stop := startFakeClockDriver(t, fc, 2*time.Second)
	defer stop()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_late",
		"--wait",
		"--poll-interval", "1s",
		"--timeout", "30s",
		"--output-format", "json",
	}, &stdout, &stderr)

	if exit == 0 {
		t.Fatalf("expected non-zero exit on later-poll timeout; stdout=%s", stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	errMsg, _ := env["error"].(string)
	if strings.Contains(errMsg, "shorter than a single API call") {
		t.Errorf("error must NOT claim 'shorter than a single API call' on attempt > 0; got %q", errMsg)
	}
}

// TestStatus_Wait_BadDuration_UsageError confirms that a malformed --timeout
// or --poll-interval value yields ErrUsage rather than a panic / silent
// fallback. cobra's DurationVar already rejects malformed values upstream,
// but this test pins the behaviour at the public CLI surface.
func TestStatus_Wait_BadDuration_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc",
		"--wait",
		"--poll-interval", "not-a-duration",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
}

// TestStatus_ProfileFlag_OverridesEnvProfile confirms the persistent --profile
// flag is threaded into config.Resolve. With URLBOX_PROFILE=different and
// --profile good, the "good" profile's secret must reach the Authorization
// header — not "different"'s.
func TestStatus_ProfileFlag_OverridesEnvProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "")
	t.Setenv("URLBOX_PROFILE", "different")
	seedConfig(t, dir, map[string]config.Profile{
		"good":      {APISecret: "sec_good"},
		"different": {APISecret: "sec_different"},
	})

	m := apitest.New(apitest.SuccessJSON(`{"status":"succeeded","renderId":"ps_x","renderUrl":"https://x/x.png"}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"status", "ps_x", "--profile", "good", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	reqs := m.Requests()
	if len(reqs) != 1 || reqs[0].Header.Get("Authorization") != "Bearer sec_good" {
		t.Errorf("Authorization=%q, want Bearer sec_good (flag --profile must beat URLBOX_PROFILE)", reqs[0].Header.Get("Authorization"))
	}
}

// Regression guard (v1.0.2): `urlbox status ""` (empty positional) must be
// rejected locally as a usage error, not forwarded to the API as an empty
// path segment. Without this guard, the empty string falls through to a 404
// "Not found" with exit 5, masking what was actually a user-input bug.
func TestStatus_EmptyArg_RejectsAsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code != 'usage': %v", env["code"])
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "empty") && !strings.Contains(errStr, "missing") {
		t.Errorf("error should mention empty/missing renderID; got %q", errStr)
	}
}

// Whitespace-only positional behaves like empty after TrimSpace.
func TestStatus_WhitespaceArg_RejectsAsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"status", "   ", "--output-format", "json"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1; stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
}

// Regression guard (v1.0.2): `--wait --timeout <very-short>` (shorter than a
// single API round trip can complete) must surface a friendly timeout
// message that names the renderID + duration — not the raw Go-internal
// "context deadline exceeded" string.
func TestStatus_Wait_TimeoutShorterThanOneCall_FriendlyError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	// Hangs longer than the --timeout so the per-poll context fires before
	// the GET can return a body. apitest.Server is for scripted responses;
	// use httptest directly for the controlled-hang behaviour.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	t.Setenv(api.EnvAPIHost, s.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_test",
		"--wait",
		"--timeout", "100ms",
		"--no-retry",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("expected non-zero exit on tiny timeout; stdout=%s", stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "100ms") {
		t.Errorf("error should mention timeout duration; got %q", errStr)
	}
	if strings.Contains(errStr, "context deadline") {
		t.Errorf("error should NOT contain jargony 'context deadline'; got %q", errStr)
	}
	if !strings.Contains(errStr, "ps_test") {
		t.Errorf("error should reference renderID; got %q", errStr)
	}
}

// Defense-in-depth: ensure the renderId is URL-escaped on the GET path.
func TestStatus_RenderIDIsPathEscaped(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	m := apitest.New(apitest.SuccessJSON(`{"status":"succeeded","renderId":"ps_abc","renderUrl":"https://x.example/x.png"}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	reqs := m.Requests()
	if len(reqs) != 1 {
		t.Fatalf("len(requests)=%d, want 1", len(reqs))
	}
	if reqs[0].Method != http.MethodGet {
		t.Errorf("Method=%q, want GET", reqs[0].Method)
	}
	wantPath := fmt.Sprintf("%s%s", api.PathStatus, "ps_abc")
	if reqs[0].Path != wantPath {
		t.Errorf("Path=%q, want %q", reqs[0].Path, wantPath)
	}
}
