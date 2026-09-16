package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const reportDTOJSON = `{"id":"rpt_x1","renderId":"01a0906a-fff5-7565-8dc5-02868f11a4fa_ps","category":"bot-detection","comment":"page shows a captcha","rects":[],"state":"open","outputUrl":null,"outputFormat":null,"resolutionMessage":null,"resolutionExampleOptions":null,"resolutionExampleUrl":null,"hasUnseenResolution":false,"createdAt":"2026-09-11T12:00:00.000Z"}`

func runReportCmd(t *testing.T, scripts []apitest.ScriptedResponse, args ...string) (stdout, stderr string, code int, srv *apitest.Server) {
	t.Helper()
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv = apitest.New(scripts...)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), code, srv
}

func TestReport_FlagsPath_PostsAndSummarises(t *testing.T) {
	stdout, stderr, code, srv := runReportCmd(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--category", "bot-detection", "--comment", "page shows a captcha",
		"--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/render-reports" {
		t.Fatalf("request: %+v", reqs[0])
	}
	var body map[string]any
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["renderId"] != "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps" ||
		body["category"] != "bot-detection" ||
		body["comment"] != "page shows a captcha" {
		t.Fatalf("body: %v", body)
	}
	if _, present := body["rects"]; present {
		t.Fatalf("rects must not be sent: %v", body)
	}
	if !strings.Contains(stdout, "Report saved for render 01a0906a-fff5-7565-8dc5-02868f11a4fa_ps") {
		t.Fatalf("summary missing: %q", stdout)
	}
}

func TestReport_JSONMode_EmitsDTOEnvelope(t *testing.T) {
	stdout, _, code, _ := runReportCmd(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--category", "other", "--comment", "x",
		"--output-format", "json")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stdout)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("envelope: %v", err)
	}
	if env["ok"] != true || env["command"] != "report" {
		t.Fatalf("envelope: %v", env)
	}
	data, _ := env["data"].(map[string]any)
	if data["id"] != "rpt_x1" || data["state"] != "open" {
		t.Fatalf("data: %v", data)
	}
}

func TestReport_QuietMode_EmitsReportID(t *testing.T) {
	stdout, _, code, _ := runReportCmd(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--category", "other", "--comment", "x",
		"--output-format", "quiet")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(stdout) != `"rpt_x1"` {
		t.Fatalf("quiet stdout=%q, want the report id", stdout)
	}
}

func TestReport_InvalidCategory_UsageErrorListsValues(t *testing.T) {
	_, stderr, code, _ := runReportCmd(t, nil,
		"report", "01a0_ps", "--category", "captcha", "--comment", "x",
		"--output-format", "text")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (usage)", code)
	}
	for _, want := range []string{"bot-detection", "login-required", "missing-content", "cookie-banner-or-popup", "render-failed", "other"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("error must list %q: %q", want, stderr)
		}
	}
}

func TestReport_EmptyComment_UsageError(t *testing.T) {
	_, _, code, _ := runReportCmd(t, nil,
		"report", "01a0_ps", "--category", "other", "--comment", "   ",
		"--output-format", "text")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
}

func TestReport_MissingFlags_NonInteractive_UsageError(t *testing.T) {
	stdout, _, code, _ := runReportCmd(t, nil,
		"report", "01a0_ps", "--output-format", "json")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (never hangs)", code)
	}
	if !strings.Contains(stdout, `"usage"`) || !strings.Contains(stdout, "--category") {
		t.Fatalf("json error envelope must name the missing flags: %q", stdout)
	}
}

func TestReport_NotLoggedIn_AuthError(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var out, errBuf bytes.Buffer
	code := Execute([]string{"report", "01a0_ps", "--category", "other", "--comment", "x", "--output-format", "text"}, &out, &errBuf)
	if code != 3 {
		t.Fatalf("exit %d, want 3 (auth)", code)
	}
	if !strings.Contains(errBuf.String(), "urlbox login") {
		t.Fatalf("auth error must carry the login hint: %q", errBuf.String())
	}
}

func TestReport_RenderNotFound_Exit5WithExpiryHint(t *testing.T) {
	notFound := apitest.ScriptedResponse{
		Status: http.StatusNotFound,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"defined":false,"code":"NOT_FOUND","status":404,"message":"Render not found"}`,
	}
	_, stderr, code, _ := runReportCmd(t, []apitest.ScriptedResponse{notFound},
		"report", "01a0906a-gone_ps", "--category", "other", "--comment", "x",
		"--output-format", "text")
	if code != 5 {
		t.Fatalf("exit %d, want 5", code)
	}
	if !strings.Contains(stderr, "Render not found") {
		t.Fatalf("API message must surface: %q", stderr)
	}
	if !strings.Contains(stderr, "60 days") {
		t.Fatalf("hint must mention render expiry: %q", stderr)
	}
}

func TestReport_LockedReport_Exit7SurfacesAPIMessage(t *testing.T) {
	locked := apitest.ScriptedResponse{
		Status: http.StatusConflict,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   `{"defined":false,"code":"CONFLICT","status":409,"message":"This report is being looked at by the team and can no longer be edited."}`,
	}
	_, stderr, code, _ := runReportCmd(t, []apitest.ScriptedResponse{locked},
		"report", "01a0_ps", "--category", "other", "--comment", "x",
		"--output-format", "text")
	if code != 7 {
		t.Fatalf("exit %d, want 7", code)
	}
	if !strings.Contains(stderr, "being looked at by the team") {
		t.Fatalf("API conflict message must surface: %q", stderr)
	}
}

func TestReport_Interactive_PromptsForCategoryAndComment(t *testing.T) {
	SetReportPromptsForTest(
		func(label string, options []string, active int) (int, error) {
			if len(options) != 6 {
				t.Fatalf("picker options: %v", options)
			}
			return 0, nil
		},
		func(title string, validate func(string) error) (string, error) {
			if validate == nil {
				t.Fatal("comment input must validate non-empty")
			}
			if err := validate(""); err == nil {
				t.Fatal("empty comment must fail validation")
			}
			return "page shows a captcha", nil
		},
	)
	t.Cleanup(ResetReportPromptsForTest)
	stdout, stderr, code, srv := runReportCmd(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0906a-fff5-7565-8dc5-02868f11a4fa_ps",
		"--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout, stderr)
	}
	var body map[string]any
	if err := json.Unmarshal(srv.Requests()[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["category"] != "bot-detection" || body["comment"] != "page shows a captcha" {
		t.Fatalf("prompted values not sent: %v", body)
	}
}

func TestReport_Interactive_FlagCategoryOnlyPromptsComment(t *testing.T) {
	picked := false
	SetReportPromptsForTest(
		func(label string, options []string, active int) (int, error) {
			picked = true
			return 0, nil
		},
		func(title string, validate func(string) error) (string, error) {
			return "left half is blank", nil
		},
	)
	t.Cleanup(ResetReportPromptsForTest)
	_, _, code, srv := runReportCmd(t,
		[]apitest.ScriptedResponse{apitest.SuccessJSON(reportDTOJSON)},
		"report", "01a0_ps", "--category", "missing-content",
		"--output-format", "text")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if picked {
		t.Fatal("category picker must not run when --category was given")
	}
	var body map[string]any
	if err := json.Unmarshal(srv.Requests()[0].Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["category"] != "missing-content" || body["comment"] != "left half is blank" {
		t.Fatalf("body: %v", body)
	}
}

func TestReport_Interactive_NonTTYStdin_FallsBackToUsage(t *testing.T) {
	ResetReportPromptsForTest()
	_, stderr, code, _ := runReportCmd(t, nil,
		"report", "01a0_ps", "--output-format", "text")
	if code != 1 {
		t.Fatalf("exit %d, want 1 (prompt.ErrNotInteractive path)", code)
	}
	if !strings.Contains(stderr, "--category") {
		t.Fatalf("usage error must name the flags: %q", stderr)
	}
}
