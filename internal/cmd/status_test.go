// internal/cmd/status_test.go
package cmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
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

// TestStatus_WaitFlag_StubbedUntilTask4 documents the placeholder behaviour:
// --wait is a registered flag, but Task 4 implements the polling loop. Until
// then, calling --wait returns a clear usage-class error so the agent isn't
// silently no-op'd.
func TestStatus_WaitFlag_StubbedUntilTask4(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"status", "ps_abc123",
		"--wait",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage stub); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code != 'usage': %v", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(strings.ToLower(errMsg), "wait") {
		t.Errorf("error should mention --wait; got %q", errMsg)
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
