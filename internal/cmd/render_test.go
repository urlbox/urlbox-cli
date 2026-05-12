package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/cmd"
)

// Note: `must` helper is defined in config_test.go (same package cmd_test).

// fakeClient satisfies api.Client without making HTTP calls. Tests inspect
// lastOpts to confirm the merged payload that was dispatched (or assert it
// stayed nil under --dry-run).
type fakeClient struct {
	lastOpts map[string]any
	resp     *api.Response
	err      error
}

func (f *fakeClient) Render(_ context.Context, opts map[string]any) (*api.Response, error) {
	f.lastOpts = opts
	return f.resp, f.err
}

func (f *fakeClient) RenderAsync(_ context.Context, opts map[string]any) (*api.Response, error) {
	f.lastOpts = opts
	return f.resp, f.err
}

func (f *fakeClient) Status(_ context.Context, _ string) (*api.Response, error) {
	return f.resp, f.err
}

func TestRender_PositionalURL_MapsToURLOption(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true, Data: map[string]any{"renderId": "ps_x"}}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://example.com", "--format", "png", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", exit, stderr.String(), stdout.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", env["data"])
	}
	if data["url"] != "https://example.com" {
		t.Errorf("url=%v, want https://example.com", data["url"])
	}
	if data["format"] != "png" {
		t.Errorf("format=%v, want png", data["format"])
	}
}

func TestRender_FlagOverridesJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"--json", `{"url":"https://json.example","format":"pdf","width":800}`,
		"--format", "png",
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["format"] != "png" {
		t.Errorf("flag should win: format=%v", data["format"])
	}
	if data["url"] != "https://json.example" {
		t.Errorf("json url should pass through: %v", data["url"])
	}
	if data["width"].(float64) != 800 {
		t.Errorf("json width should pass through: %v", data["width"])
	}
}

func TestRender_JSON_Stdin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	cmd.SetStdinForTest(strings.NewReader(`{"url":"https://stdin.example","format":"pdf"}`))
	t.Cleanup(cmd.ResetStdinForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "--json", "-", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["data"].(map[string]any)["url"] != "https://stdin.example" {
		t.Errorf("stdin url not picked up: %v", env["data"])
	}
}

func TestRender_JSON_File(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	f := filepath.Join(dir, "opts.json")
	if err := os.WriteFile(f, []byte(`{"url":"https://file.example"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "--json", "@" + f, "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["data"].(map[string]any)["url"] != "https://file.example" {
		t.Errorf("file url not picked up: %v", env["data"])
	}
}

func TestRender_NoURL_NoJSON_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "--output-format", "json"}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "url") {
		t.Errorf("error should mention url: %v", env["error"])
	}
}

// v0.9.0: typed-flag enum values get strict client-side validation at the
// Cobra layer (this is the explicit contract — fast local feedback). The
// schema's enum list is the source of truth.
func TestRender_TypedFlag_WaitUntil_InvalidEnum_ErrorsLocally(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"https://example.com",
		"--wait-until", "totally_made_up",
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 2 {
		t.Fatalf("exit=%d, want 2 (validation); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "wait-until") {
		t.Errorf("error should mention --wait-until; got: %q", errStr)
	}
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, "Allowed:") {
		t.Errorf("hint should list allowed values; got: %q", hint)
	}
}

// Same contract for --format.
func TestRender_TypedFlag_Format_InvalidEnum_ErrorsLocally(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"https://example.com",
		"--format", "xyzzy",
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 2 {
		t.Fatalf("exit=%d, want 2 (validation); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "format") {
		t.Errorf("error should mention --format; got: %q", errStr)
	}
}

// Counterpart: --format passed via --json (NOT a typed flag) is passthrough.
// Confirms the contract split: typed flags are gated, --json is not.
func TestRender_JSONFormat_BadEnum_PassesThroughToAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"--json", `{"url":"https://example.com","format":"xyzzy"}`,
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d, want 0 (passthrough); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if data["format"] != "xyzzy" {
		t.Errorf("expected format=xyzzy verbatim; got %v", data["format"])
	}
}

// v0.9.0 schema-as-docs contract: known-key bad type passes through to the
// API. The CLI no longer gates type errors locally; the API returns
// structured InvalidOptionsError (Zod tree). Local --dry-run echoes the
// payload verbatim including format:42.
func TestRender_KnownKeyBadType_PassesThroughToAPI(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"--json", `{"url":"https://example.com","format":42}`,
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d, want 0 (passthrough); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if data["format"] != float64(42) {
		t.Errorf("expected data.format=42 (verbatim passthrough); got %v", data["format"])
	}
}

// v0.9.0 schema-as-docs contract: unknown key with a fuzzy match emits a
// stderr warning but the request still goes through verbatim — agent reads
// the warning, decides whether to re-run with the suggested fix.
func TestRender_FuzzyCorrection_WarnsAndPassesThrough(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"--json", `{"url":"https://example.com","fromat":"png"}`,
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d, want 0 (passthrough); stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}
	// Stderr should carry the warning.
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("expected 'warning:' prefix on stderr; got: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "fromat") || !strings.Contains(stderr.String(), "format") {
		t.Errorf("expected stderr to mention 'fromat' and suggest 'format'; got: %s", stderr.String())
	}
	// Payload passes through verbatim (key NOT auto-corrected).
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if data["fromat"] != "png" {
		t.Errorf("expected data.fromat=png (verbatim, not auto-corrected); got %v", data["fromat"])
	}
}

func TestRender_BadJSON_UsageOrValidationError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render",
		"--json", `{"not closed`,
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 && exit != 2 {
		t.Fatalf("exit=%d, want 1 or 2; stdout=%s", exit, stdout.String())
	}
}

func TestRender_BothPositionalAndJSON_FlagWins(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://positional.example",
		"--json", `{"url":"https://json.example"}`,
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	url := env["data"].(map[string]any)["url"]
	if url != "https://positional.example" {
		t.Errorf("url=%v, want https://positional.example (positional is in flags tier, last writer wins)", url)
	}
}

// Confirm --dry-run doesn't reach the client.
func TestRender_DryRun_DoesNotInvokeClient(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"render", "https://example.com", "--dry-run", "--output-format", "json"}, &stdout, &stderr)
	if fc.lastOpts != nil {
		t.Errorf("client was called on --dry-run: %v", fc.lastOpts)
	}
}

// Hard guarantee: --dry-run must NEVER make an HTTP call. Point the env at
// a server that fails the test on any request, then run --dry-run.
func TestRender_DryRun_DoesNotHitNetwork(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := apitest.NeverHitServer(t)
	t.Cleanup(s.Close)
	t.Setenv(api.EnvAPIHost, s.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
}

// Same hard guarantee for --curl: must NEVER make an HTTP call.
func TestRender_Curl_DoesNotHitNetwork(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := apitest.NeverHitServer(t)
	t.Cleanup(s.Close)
	t.Setenv(api.EnvAPIHost, s.URL)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--curl", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	curl, _ := data["curl"].(string)
	if !strings.Contains(curl, "curl -X POST") {
		t.Errorf("curl command malformed: %q", curl)
	}
	if !strings.Contains(curl, "$URLBOX_API_SECRET") {
		t.Errorf("curl should reference $URLBOX_API_SECRET (redacted secret); got: %q", curl)
	}
	if strings.Contains(curl, "ubx_") || strings.Contains(curl, "sec_") {
		t.Errorf("curl appears to leak a real secret: %q", curl)
	}
}

func TestRender_Curl_AsyncFlag_HitsAsyncPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{
		"render", "https://example.com",
		"--curl", "--async", "--output-format", "json",
	}, &stdout, &stderr)
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nout: %s", err, stdout.String())
	}
	curl := env["data"].(map[string]any)["curl"].(string)
	if !strings.Contains(curl, "/v1/screenshot/async") {
		t.Errorf("--curl --async should target async endpoint; got: %s", curl)
	}
}

func TestRender_Preset_FillsDefaults_OverridableByJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--preset", "mobile",
		"--json", `{"width":500}`, // overrides preset's width=375
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["width"].(float64) != 500 {
		t.Errorf("width=%v, want 500 (--json overrides preset)", data["width"])
	}
	if data["height"].(float64) != 812 {
		t.Errorf("height=%v, want 812 (preset default; --json didn't touch height)", data["height"])
	}
}

func TestRender_Preset_OverridableByFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--preset", "desktop",
		"--width", "2560", // overrides preset's 1920
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["width"].(float64) != 2560 {
		t.Errorf("width=%v, want 2560 (flag overrides preset)", data["width"])
	}
	if data["height"].(float64) != 1080 {
		t.Errorf("height=%v, want 1080 (preset default; flag didn't touch height)", data["height"])
	}
}

func TestRender_PresetArticle_FillsNewsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fc := &fakeClient{resp: &api.Response{OK: true}}
	cmd.SetClientForTest(fc)
	t.Cleanup(cmd.ResetClientForTest)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://news.example",
		"--preset", "article",
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data := env["data"].(map[string]any)
	if data["block_ads"] != true {
		t.Errorf("block_ads=%v, want true", data["block_ads"])
	}
	if data["retina"] != true {
		t.Errorf("retina=%v, want true", data["retina"])
	}
	if data["wait_until"] != "mostrequestsfinished" {
		t.Errorf("wait_until=%v, want mostrequestsfinished", data["wait_until"])
	}
}

func TestRender_UnknownPreset_UsageError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--preset", "nonsense",
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage); stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["code"] != "usage" {
		t.Errorf("code=%v", env["code"])
	}
	hint, _ := env["hint"].(string)
	for _, name := range []string{"mobile", "desktop", "pdf-a4"} {
		if !strings.Contains(hint, name) {
			t.Errorf("hint should list available preset %q; got %q", name, hint)
		}
	}
}

// fakeOpener captures the URLs --open passes to the browser layer.
type fakeOpener struct{ urls []string }

func (f *fakeOpener) Open(url string) error { f.urls = append(f.urls, url); return nil }

func TestRender_FullPipeline_HappyPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 245632,
		"renderTime": 1234,
		"width": 1920,
		"height": 1080
	}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())
	// Need a secret in env so config.Resolve gives us non-empty APISecret.
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--format", "png",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["ok"] != true {
		t.Errorf("ok=%v", env["ok"])
	}
	data := env["data"].(map[string]any)
	if data["renderUrl"] != "https://renders.urlbox.com/x.png" {
		t.Errorf("renderUrl=%v", data["renderUrl"])
	}
	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Errorf("breadcrumbs empty; want save suggestion")
	}
}

func TestRender_OutputSavesFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	// Two servers: one for the API, one for the render bytes.
	blob := []byte("\x89PNG\r\n\x1a\n--fake-png-bytes--")
	rs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(blob)
	}))
	t.Cleanup(rs.Close)

	apiSrv := apitest.New(apitest.SuccessJSON(fmt.Sprintf(`{
		"renderUrl": %q,
		"size": %d,
		"renderTime": 1000,
		"width": 800,
		"height": 600
	}`, rs.URL+"/x.png", len(blob))))
	t.Cleanup(apiSrv.Close)
	t.Setenv(api.EnvAPIHost, apiSrv.URL())

	tmpDir := t.TempDir()
	oldwd, _ := os.Getwd()
	must(t, os.Chdir(tmpDir))
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	// Capture the *canonical* cwd: macOS resolves /var to /private/var on
	// chdir, and resolveOutputPath uses that form. Comparing against the
	// raw t.TempDir() path would fail on macOS.
	canonicalCWD, _ := os.Getwd()

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--output", "out.png",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	saved, err := os.ReadFile(filepath.Join(canonicalCWD, "out.png"))
	if err != nil {
		t.Fatalf("file not saved: %v", err)
	}
	if !bytes.Equal(saved, blob) {
		t.Errorf("saved bytes mismatch")
	}

	var env map[string]any
	if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
		t.Fatalf("not JSON: %v", jerr)
	}
	data := env["data"].(map[string]any)
	want := filepath.Join(canonicalCWD, "out.png")
	if data["savedTo"] != want {
		t.Errorf("savedTo=%v, want %v", data["savedTo"], want)
	}
}

func TestRender_OutputPathEscape_ValidationError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	m := apitest.New(apitest.SuccessJSON(`{"renderUrl":"https://renders.urlbox.com/x.png","size":100}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--output", "../escape.png",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 2 {
		t.Errorf("exit=%d, want 2 (validation); stdout=%s", exit, stdout.String())
	}
}

func TestRender_Open_InvokesOpener(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 100
	}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	fo := &fakeOpener{}
	cmd.SetOpenerForTest(fo)
	t.Cleanup(cmd.ResetOpenerForTest)

	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{
		"render", "https://example.com",
		"--open", "--output-format", "json",
	}, &stdout, &stderr)
	if len(fo.urls) != 1 || fo.urls[0] != "https://renders.urlbox.com/x.png" {
		t.Errorf("opener urls=%v, want [https://renders.urlbox.com/x.png]", fo.urls)
	}
}

func TestRender_Async_BreadcrumbsIncludeStatus(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	m := apitest.New(apitest.SuccessJSON(`{
		"status": "created",
		"renderId": "ps_abc",
		"statusUrl": "https://api.urlbox.com/v1/render/ps_abc"
	}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{
		"render", "https://example.com",
		"--async", "--output-format", "json",
	}, &stdout, &stderr)
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Fatalf("async should include breadcrumbs")
	}
	bc := bcs[0].(map[string]any)
	if bc["action"] != "status" {
		t.Errorf("first breadcrumb action=%v, want status", bc["action"])
	}
	cmdStr, _ := bc["cmd"].(string)
	if !strings.Contains(cmdStr, "ps_abc") {
		t.Errorf("status breadcrumb should include renderId; got %q", cmdStr)
	}
}

// SURFACE.txt regression guard: --api-secret flag must be on render (per-call override).
func TestRender_HasAPISecretFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"render", "--help"}, &stdout, &stderr)
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "--api-secret") {
		t.Error("--api-secret flag missing from render --help")
	}
	if strings.Contains(help, "--api-key") {
		t.Error("--api-key should not be on render (renamed in v0.6.0)")
	}
}

func TestRender_UpstreamError_FlagsInSummary(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	m := apitest.New(apitest.SuccessJSON(`{
		"renderUrl": "https://renders.urlbox.com/x.png",
		"size": 176000,
		"response": {"statusCode": 401}
	}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://reuters.example", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	summary, _ := env["summary"].(string)
	if !strings.Contains(summary, "upstream") || !strings.Contains(summary, "401") {
		t.Errorf("summary should warn about upstream 401; got %q", summary)
	}
	data := env["data"].(map[string]any)
	if data["upstreamOk"] != false {
		t.Errorf("data.upstreamOk=%v, want false", data["upstreamOk"])
	}
}

func TestRender_Timeout_FastFails_WithRecoveryHint(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	t.Cleanup(slow.Close)
	t.Setenv(api.EnvAPIHost, slow.URL)

	var stdout, stderr bytes.Buffer
	start := time.Now()
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--timeout", "100ms",
		"--no-retry",
		"--output-format", "json",
	}, &stdout, &stderr)
	elapsed := time.Since(start)

	if exit != 11 {
		t.Errorf("exit=%d, want 11 (network/timeout class)", exit)
	}
	if elapsed > 1*time.Second {
		t.Errorf("elapsed=%v, want <1s (no auto-retry on timeout)", elapsed)
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "timeout" {
		t.Fatalf("code=%v, want \"timeout\"", env["code"])
	}
	hint, _ := env["hint"].(string)
	for _, want := range []string{"retry the same command", "--timeout", "--async"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q; got %q", want, hint)
		}
	}
}

func TestRender_TimeoutFlag_DefaultDocumentedInHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"render", "--help"}, &stdout, &stderr)
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "--timeout") {
		t.Errorf("--help missing --timeout flag")
	}
	if !strings.Contains(help, "1m0s") && !strings.Contains(help, "60s") {
		t.Errorf("--help should document the 60s default; got %q", help)
	}
}

// Regression guard (v1.0.2): --dry-run with an invalid --profile must fail
// fast with code "not_found" instead of silently succeeding. Profile
// validation has to happen BEFORE the dry-run early return so scripts that
// use --dry-run for pre-flight validation catch a bad profile name.
func TestRender_DryRun_InvalidProfile_StillFails(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("URLBOX_API_SECRET", "")
	if err := os.MkdirAll(filepath.Join(tmp, "urlbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := `{"default_profile":"good","profiles":{"good":{"api_secret":"sec"}}}`
	if err := os.WriteFile(filepath.Join(tmp, "urlbox", "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--profile", "nonexistent",
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)

	if exit == 0 {
		t.Fatalf("expected non-zero exit (profile must validate even on --dry-run); got 0\nstdout=%s", stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "not_found" {
		t.Errorf("code != 'not_found': %v", env["code"])
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "nonexistent") {
		t.Errorf("error should mention bad profile name; got %q", errStr)
	}
}

// Regression guard: --wait-until's help text must list the real enum values
// from the schema, not the Puppeteer-style guesses we shipped in v0.7.0.
func TestRender_WaitUntilHelp_ListsRealEnumValues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"render", "--help"}, &stdout, &stderr)
	help := stdout.String() + stderr.String()
	for _, want := range []string{"domloaded", "mostrequestsfinished"} {
		if !strings.Contains(help, want) {
			t.Errorf("--help missing %q (the API's real enum value)", want)
		}
	}
	for _, badGuess := range []string{"networkidle0", "domcontentloaded"} {
		if strings.Contains(help, badGuess) {
			t.Errorf("--help still contains Puppeteer-style %q (the API rejects this)", badGuess)
		}
	}
}

// TestRender_APISecretStdin_ConflictsWithJSONStdin pins that --api-secret-stdin
// and --json - cannot share stdin; the runRender entry rejects the combo
// before either reader touches stdin. Round 1 S-C2 follow-up.
func TestRender_APISecretStdin_ConflictsWithJSONStdin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "--json", "-", "--api-secret-stdin",
	}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on stdin double-claim")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "stdin") {
		t.Errorf("error should mention stdin conflict; got %q", errStr)
	}
}

// TestRender_DryRun_ValidatesOutputSandbox pins Round 4 M1: --dry-run
// must catch --output paths that escape the CWD sandbox. Before this fix,
// "ok: true, payload validated" came back for paths the real run would
// reject — defeating dry-run as a pre-flight tool.
func TestRender_DryRun_ValidatesOutputSandbox(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--output", "/etc/passwd",
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 2 {
		t.Errorf("dry-run with sandbox-escaping --output should exit 2 (validation); got %d. stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "validation" {
		t.Errorf("code=%v, want validation", env["code"])
	}
}

// TestRender_OutputPath_WritabilityPreflight pins Round 4 M6: when
// --output points at a directory the user can't write, the failure must
// be detected BEFORE the API call (no wasted credit) and surfaced as
// ErrValidation (not ErrServer — that misclassified the user error as
// our server's fault).
func TestRender_OutputPath_WritabilityPreflight(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	// Set up a read-only sub-directory inside a writable parent. macOS
	// honors 0o500 for the owner; Linux too. Cleanup must chmod back to
	// 0o700 so t.TempDir's RemoveAll can sweep it.
	root := t.TempDir()
	readonly := filepath.Join(root, "ro")
	if err := os.Mkdir(readonly, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonly, 0o700) })

	// Use a server that would respond OK to detect whether the API was
	// hit. If the preflight works, no request reaches this handler.
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"renderUrl":"https://renders.urlbox.invalid/x.png","size":100}`))
	}))
	defer srv.Close()
	t.Setenv(api.EnvAPIHost, srv.URL)

	prevDir, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--output", "ro/cantwrite.png",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 2 {
		t.Errorf("exit=%d, want 2 (validation). stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "validation" {
		t.Errorf("code=%v, want validation (not server)", env["code"])
	}
	if hits != 0 {
		t.Errorf("API was hit %d times despite unwritable --output; preflight failed", hits)
	}
}

// TestRender_NumericFlag_BeyondJSONSafeInt_Errors pins Round 4 M2:
// width/height/etc. accepted Go int64 values past 2^53 and JSON-marshaled
// them as float64 — silently rounding to a different value than the user
// passed. The schema documents [-9007199254740991, 9007199254740991]; the
// CLI now enforces it on typed flags before payload assembly.
func TestRender_NumericFlag_BeyondJSONSafeInt_Errors(t *testing.T) {
	cases := []struct {
		name string
		flag string
		val  string
	}{
		{"width too large", "--width", "9007199254740993"},
		{"width too negative", "--width", "-9007199254740993"},
		{"height too large", "--height", "9223372036854775807"},
		{"delay too large", "--delay", "9007199254740993"},
		{"quality too large", "--quality", "9007199254740993"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("URLBOX_API_SECRET", "sec_test")

			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{
				"render", "https://example.com",
				c.flag, c.val,
				"--dry-run",
				"--output-format", "json",
			}, &stdout, &stderr)
			if exit != 2 {
				t.Fatalf("%s=%s should exit 2 (validation); got %d. stdout=%s", c.flag, c.val, exit, stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "validation" {
				t.Errorf("code=%v, want validation", env["code"])
			}
		})
	}
}

// TestRender_NumericFlag_AtBoundary_Allowed pins that exactly 2^53-1 is
// permitted — it's the documented schema max.
func TestRender_NumericFlag_AtBoundary_Allowed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--width", "9007199254740991",
		"--dry-run",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("boundary value should succeed; got exit %d, stdout=%s", exit, stdout.String())
	}
}

// TestRender_JSONPath_BeyondJSONSafeInt_Errors pins Round 5 Adv-1 (High):
// the M2 flag-parser fix doesn't apply to values passed through --json,
// even though the flag-parser's own error message warns about exactly
// this rounding. The dry-run output silently substituted
// 9007199254740993 → 9007199254740992 (float64 rounding).
//
// Fix: re-run the int-range check after JSON parse + merge, on the
// merged numeric option keys (width, height, delay, quality).
func TestRender_JSONPath_BeyondJSONSafeInt_Errors(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"width over 2^53", `{"url":"https://example.com","width":9007199254740993}`},
		{"width negative over 2^53", `{"url":"https://example.com","width":-9007199254740993}`},
		{"height over 2^53", `{"url":"https://example.com","height":9007199254740993}`},
		{"delay over 2^53", `{"url":"https://example.com","delay":9007199254740993}`},
		{"quality over 2^53", `{"url":"https://example.com","quality":9007199254740993}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("URLBOX_API_SECRET", "sec_test")

			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{
				"render", "--json", c.json,
				"--dry-run",
				"--output-format", "json",
			}, &stdout, &stderr)
			if exit != 2 {
				t.Fatalf("%s should exit 2 (validation); got %d. stdout=%s", c.name, exit, stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "validation" {
				t.Errorf("code=%v, want validation", env["code"])
			}
		})
	}
}

// TestRender_JSONPath_AtBoundary_Allowed pins that exactly 2^53-1 on
// the --json path is accepted (boundary is the documented schema max,
// not one-past).
func TestRender_JSONPath_AtBoundary_Allowed(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "--json", `{"url":"https://example.com","width":9007199254740991}`,
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("boundary on JSON path should succeed; got exit %d, stdout=%s", exit, stdout.String())
	}
}

// TestRender_JSONPath_FloatValue_PassesThrough pins that a non-integer
// numeric in --json (e.g. {"width": 1.5}) is NOT trapped by the int-range
// check — the API will reject it as it sees fit. Schema-as-docs applies
// to type validation; we only intervene on silent rounding.
func TestRender_JSONPath_FloatValue_PassesThrough(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "--json", `{"url":"https://example.com","width":1.5}`,
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("float width should pass --dry-run; got exit %d, stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	if w, _ := data["width"].(float64); w != 1.5 {
		t.Errorf("width should be 1.5 in envelope; got %v", data["width"])
	}
}

// TestRender_JSONPath_NestedSlice_NoStrayJSONNumber pins that the
// json.Number -> float64 conversion recurses into arrays. Previously,
// {"tags":[1,2,3]} would leave the inner values as json.Number; the
// envelope still rendered them correctly thanks to json.Number's
// MarshalJSON, but the in-memory map had inconsistent value types.
func TestRender_JSONPath_NestedSlice_NoStrayJSONNumber(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "--json", `{"url":"https://example.com","tags":[1,2,3]}`,
		"--dry-run", "--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("nested-slice payload should pass; got %d stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	tags, _ := data["tags"].([]any)
	if len(tags) != 3 {
		t.Fatalf("tags should round-trip as a 3-element array; got %v", data["tags"])
	}
	// Each element should be a float64 (per the converter), not a string.
	for i, v := range tags {
		if _, ok := v.(float64); !ok {
			t.Errorf("tags[%d] = %v (%T), want float64", i, v, v)
		}
	}
}

// TestRender_QuietMode_EmitsSingleUsefulValue pins Round 5 First-3 /
// Power-1: render --output-format quiet used to dump the full data
// JSON. Other quiet-mode commands (link, config get, config path)
// emit a single bare value suitable for piping. Render should follow
// suit, contextually:
//
//   - sync + --output -> savedTo (the local file path the user just got)
//   - sync + no --output -> renderUrl (the hosted artifact URL)
//   - async -> renderId (for later urlbox status calls)
func TestRender_QuietMode_EmitsSingleUsefulValue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	t.Run("sync no output -> bare renderUrl", func(t *testing.T) {
		m := apitest.New(apitest.SuccessJSON(`{"renderUrl":"https://renders.urlbox.com/quiet_sync.png","size":100}`))
		t.Cleanup(m.Close)
		t.Setenv(api.EnvAPIHost, m.URL())

		var stdout, stderr bytes.Buffer
		exit := cmd.Execute([]string{"render", "https://example.com", "--output-format", "quiet"}, &stdout, &stderr)
		if exit != 0 {
			t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
		}
		out := strings.TrimSpace(stdout.String())
		if out != "https://renders.urlbox.com/quiet_sync.png" {
			t.Errorf("got %q, want bare renderUrl", out)
		}
		// Hard no on JSON braces in quiet mode.
		if strings.Contains(out, "{") {
			t.Errorf("quiet output must not contain JSON object; got %q", out)
		}
	})

	t.Run("async -> bare renderId", func(t *testing.T) {
		m := apitest.New(apitest.SuccessJSON(`{"renderId":"ren_async_quiet_abc","statusUrl":"https://api.urlbox.com/v1/status/ren_async_quiet_abc","status":"created"}`))
		t.Cleanup(m.Close)
		t.Setenv(api.EnvAPIHost, m.URL())

		var stdout, stderr bytes.Buffer
		exit := cmd.Execute([]string{"render", "https://example.com", "--async", "--output-format", "quiet"}, &stdout, &stderr)
		if exit != 0 {
			t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
		}
		out := strings.TrimSpace(stdout.String())
		if out != "ren_async_quiet_abc" {
			t.Errorf("got %q, want bare renderId", out)
		}
	})
}

// TestRender_TextMode_SummaryOnly pins Round 5 Power-2: --output-format
// text used to print the green ✓ summary AND a raw JSON dump of .data.
// "text" should be the human-readable format — summary only. Users who
// want JSON should pass --output-format json.
func TestRender_TextMode_SummaryOnly(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	m := apitest.New(apitest.SuccessJSON(`{"renderUrl":"https://renders.urlbox.com/text.png","size":100}`))
	t.Cleanup(m.Close)
	t.Setenv(api.EnvAPIHost, m.URL())

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://example.com", "--output-format", "text"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "{") || strings.Contains(out, "\"renderUrl\"") {
		t.Errorf("text mode must not emit JSON; got:\n%s", out)
	}
	// The summary line is what we want.
	if !strings.Contains(out, "Render") && !strings.Contains(out, "✓") {
		t.Errorf("text mode should emit a human summary line; got:\n%s", out)
	}
}

// TestRender_DryRunWithCurl_WarnsAboutSilentDrop pins Round 5 Adv-3:
// when --dry-run AND --curl are passed together, --dry-run wins
// silently. The agent's complaint was "the user has no signal that
// --curl was dropped". Now: a breadcrumb in the dry-run envelope
// explicitly names the dropped flag.
func TestRender_DryRunWithCurl_WarnsAboutSilentDrop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--dry-run", "--curl",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "--curl") {
		t.Errorf("dry-run envelope should warn that --curl was dropped; got:\n%s", out)
	}
}

// TestRender_DryRunWithOutput_WarnsAboutSilentDrop mirrors the above for
// --dry-run --output. Output silently doesn't fire on dry-run.
func TestRender_DryRunWithOutput_WarnsAboutSilentDrop(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	cwd := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--dry-run", "--output", "shot.png",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "--output") {
		t.Errorf("dry-run envelope should warn that --output was dropped; got:\n%s", out)
	}
}

// TestRender_NegativeTimeout_Errors pins Round 5 Adv-5: --timeout -5s
// used to produce the nonsense diagnostic "Render timed out after -5s".
// Negative durations should be rejected at parse time with a clear
// usage error.
func TestRender_NegativeTimeout_Errors(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"render", "https://example.com",
		"--timeout", "-5s",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("exit=%d, want 1 (usage); stdout=%s", exit, stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	if !strings.Contains(env["error"].(string), "positive") && !strings.Contains(env["error"].(string), "negative") {
		t.Errorf("error should mention positive/negative; got %q", env["error"])
	}
}

// TestRender_JSONEmptyURL_Errors pins Round 5 Adv-7: --json '{"url":""}'
// and --json '{"url":null}' bypassed the "missing url" guard because
// it only checked the KEY's presence, not the value. Empty / null URL
// is structurally equivalent to "no URL" — reject both.
func TestRender_JSONEmptyURL_Errors(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"empty string url", `{"url":""}`},
		{"null url", `{"url":null}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("URLBOX_API_SECRET", "sec_test")

			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{
				"render", "--json", c.json,
				"--dry-run", "--output-format", "json",
			}, &stdout, &stderr)
			if exit != 1 {
				t.Fatalf("exit=%d, want 1 (usage); stdout=%s", exit, stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "usage" {
				t.Errorf("code=%v, want usage", env["code"])
			}
			if !strings.Contains(env["error"].(string), "url") {
				t.Errorf("error should mention url; got %q", env["error"])
			}
		})
	}
}
