package cmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
)

func runDoctor(t *testing.T, args ...string) (env map[string]any, exit int, stdoutStr, stderrStr string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	full := append([]string{"doctor", "--output-format", "json"}, args...)
	exit = cmd.Execute(full, &stdout, &stderr)
	stdoutStr, stderrStr = stdout.String(), stderr.String()
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("doctor stdout not JSON: %v\nstdout=%s", err, stdoutStr)
	}
	return env, exit, stdoutStr, stderrStr
}

func extractCheck(t *testing.T, env map[string]any, name string) map[string]any {
	t.Helper()
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data not a map: %v", env["data"])
	}
	checks, ok := data["checks"].([]any)
	if !ok {
		t.Fatalf("checks not an array: %v", data["checks"])
	}
	for _, c := range checks {
		m, _ := c.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	t.Fatalf("check %q not found in %v", name, checks)
	return nil
}

func hasCheck(t *testing.T, env map[string]any, name string) bool {
	t.Helper()
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data not a map: %v", env["data"])
	}
	checks, ok := data["checks"].([]any)
	if !ok {
		t.Fatalf("checks not an array: %v", data["checks"])
	}
	for _, c := range checks {
		m, _ := c.(map[string]any)
		if m["name"] == name {
			return true
		}
	}
	return false
}

func TestDoctor_AllChecksPass_Exit0(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/get-session" {
			_, _ = w.Write([]byte(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationPublicId":"org_acme"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "")
	t.Setenv("URLBOX_API_HOST", srv.URL)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {
			APISecret:     "sec_test",
			SessionToken:  "sess_tok",
			ActiveOrg:     "org_acme",
			ActiveProject: "proj_1",
		},
	})

	env, exit, _, _ := runDoctor(t)
	if exit != 0 {
		t.Fatalf("exit=%d env=%v", exit, env)
	}
	if env["ok"] != true {
		t.Fatalf("ok=%v", env["ok"])
	}

	for _, name := range []string{"version", "install_method", "config_file", "session", "active_org", "active_project", "render_credential", "dns", "api_reachable"} {
		_ = extractCheck(t, env, name)
	}

	rc := extractCheck(t, env, "render_credential")
	if rc["status"] != "ok" {
		t.Fatalf("render_credential status = %v want ok", rc["status"])
	}
	if msg, _ := rc["message"].(string); msg != "valid (file)" {
		t.Fatalf("render_credential message = %q, want \"valid (file)\"", msg)
	}
}

func TestDoctor_NoRenderCredential_FailsRenderCredentialCheck(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "render_credential")
	if c["status"] != "fail" {
		t.Fatalf("render_credential status = %v want fail", c["status"])
	}
	if hint, _ := c["hint"].(string); !strings.Contains(hint, "urlbox login") {
		t.Fatalf("render_credential hint should point at login; got %q", hint)
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when checks fail")
	}
}

func TestDoctor_LoggedOut_FailsSessionCheck(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "session")
	if c["status"] != "fail" {
		t.Fatalf("session status = %v want fail", c["status"])
	}
	if hint, _ := c["hint"].(string); hint != "Run `urlbox login` to sign in." {
		t.Fatalf("session hint = %q, want the unified login hint", hint)
	}
	for _, name := range []string{"active_org", "active_project"} {
		if extractCheck(t, env, name)["status"] != "fail" {
			t.Fatalf("%s should fail when logged out", name)
		}
	}
	if hasCheck(t, env, "auth") {
		t.Fatal("auth check name should no longer appear — folded into render_credential")
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when logged out")
	}
}

// TestDoctor_CredentialRejected_FailsRenderCredential pins the folded
// behaviour: a present secret the API rejects (401-class) marks
// render_credential fail with "credential invalid" and the pinned login
// hint, and drives the auth-class exit code (3).
func TestDoctor_CredentialRejected_FailsRenderCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_bad")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "render_credential")
	if c["status"] != "fail" {
		t.Fatalf("render_credential status = %v want fail", c["status"])
	}
	if msg, _ := c["message"].(string); msg != "credential invalid" {
		t.Fatalf("render_credential message = %q, want \"credential invalid\"", msg)
	}
	if hint, _ := c["hint"].(string); hint != "Run `urlbox login` to sign in." {
		t.Fatalf("render_credential hint = %q, want the pinned login hint", hint)
	}
	if exit != 3 {
		t.Fatalf("exit = %d, want 3 (auth class)", exit)
	}
}

// TestDoctor_CredentialValid_PassesRenderCredential pins the positive
// fold: a present secret the API accepts (2xx) marks render_credential ✓
// with the source-tagged "valid (env)" message.
func TestDoctor_CredentialValid_PassesRenderCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/get-session" {
			_, _ = w.Write([]byte(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationPublicId":"org_acme"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "sec_good")
	t.Setenv("URLBOX_API_HOST", srv.URL)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {
			SessionToken:  "sess_tok",
			ActiveOrg:     "org_acme",
			ActiveProject: "proj_1",
		},
	})

	env, exit, _, _ := runDoctor(t)
	if exit != 0 {
		t.Fatalf("exit=%d env=%v", exit, env)
	}
	c := extractCheck(t, env, "render_credential")
	if c["status"] != "ok" {
		t.Fatalf("render_credential status = %v want ok", c["status"])
	}
	if msg, _ := c["message"].(string); msg != "valid (env)" {
		t.Fatalf("render_credential message = %q, want \"valid (env)\"", msg)
	}
}

// TestDoctor_CredentialBadRequest_FailsRenderCredential pins Round 4 H2
// through the fold: the probe previously only treated 401/403/5xx as
// failure. A real-world 400 from /v1/user/me with body
// {"error":{"code":"ApiKeyNotFound",...}} fell into the default arm and
// was reported valid — a critical false-positive that let CI green-light
// a broken secret.
func TestDoctor_CredentialBadRequest_FailsRenderCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"ApiKeyNotFound","message":"Api Key does not exist"}}`))
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "ubx_sk_fakebutwellformed")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "render_credential")
	if c["status"] != "fail" {
		t.Fatalf("render_credential on HTTP 400 should fail; got %v (full check: %v)", c["status"], c)
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when render_credential fails")
	}
}

// TestDoctor_CredentialNotFound_FailsRenderCredential pins another
// non-2xx, non-401/403 case — 404 on /v1/user/me should not be valid
// either.
func TestDoctor_CredentialNotFound_FailsRenderCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "ubx_sk_anything")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "render_credential")
	if c["status"] != "fail" {
		t.Fatalf("render_credential on HTTP 404 should fail; got %v", c["status"])
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when render_credential fails")
	}
}

// Regression guard (v1.0.2): when any check fails the envelope's `ok` field
// must be false. Exit code is already 10 (correct), but consumers parsing
// JSON would otherwise see `ok: true` alongside failed checks — a contract
// violation that misleads automation.
func TestDoctor_FailedChecks_EnvelopeOkIsFalse(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")
	t.Setenv("URLBOX_API_HOST", "https://api.urlbox.invalid")

	env, exit, _, _ := runDoctor(t)
	if exit == 0 {
		t.Fatal("expected non-zero exit on failed checks")
	}
	if env["ok"] != false {
		t.Errorf("envelope ok should be false when checks fail; got %v", env["ok"])
	}
}

func TestDoctor_HasBreadcrumbs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, _, _, _ := runDoctor(t)
	if bcs, _ := env["breadcrumbs"].([]any); len(bcs) == 0 {
		t.Fatalf("expected breadcrumbs, got %v", env["breadcrumbs"])
	}
}

// TestDoctor_HonorsProfileFlag pins Round 6 class-fix: doctor used to
// silently ignore --profile / URLBOX_PROFILE and always look at the
// default profile's secret. Now: profile resolution is uniform with
// every other command — unknown name errors, valid name targets that
// profile's credentials.
func TestDoctor_HonorsProfileFlag_UnknownErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "") // force file lookup
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xx"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--profile", "ghost", "doctor", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("doctor with --profile ghost should error; got exit 0 stdout=%s", stdout.String())
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" && env["code"] != "not_found" {
		t.Errorf("code=%v, want usage or not_found", env["code"])
	}
	errStr, _ := env["error"].(string)
	if !strings.Contains(errStr, "ghost") {
		t.Errorf("error should name the rejected profile; got %q", errStr)
	}
}

func TestDoctor_HonorsEnvProfile_UnknownErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "")
	t.Setenv("URLBOX_PROFILE", "ghost")
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xx"},
	})

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"doctor", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("doctor with URLBOX_PROFILE=ghost should error; got exit 0 stdout=%s", stdout.String())
	}
}

// TestDoctor_HonorsProfileFlag_ValidTargetsThatProfile pins the positive
// case for the Round 6 Z class-fix: --profile work makes doctor check
// the work profile's secret, not default's.
func TestDoctor_HonorsProfileFlag_ValidTargetsThatProfile(t *testing.T) {
	// Use a httptest server so the auth check is hermetic. The handler
	// records the Authorization header so we can prove the right
	// profile's secret was used.
	var seenAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/user/me" {
			seenAuth = r.Header.Get("Authorization")
		}
		if r.URL.Path == "/v1/auth/get-session" {
			_, _ = w.Write([]byte(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationPublicId":"org_acme"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "") // force file lookup
	t.Setenv("URLBOX_API_HOST", srv.URL)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xx"},
		"work": {
			APISecret:     "sec_work_yy",
			SessionToken:  "sess_work",
			ActiveOrg:     "org_acme",
			ActiveProject: "proj_1",
		},
	})

	env, exit, _, _ := runDoctor(t, "--profile", "work")
	if exit != 0 {
		t.Fatalf("exit=%d env=%v", exit, env)
	}
	if seenAuth != "Bearer sec_work_yy" {
		t.Errorf("auth check used wrong profile's secret; saw %q, want Bearer sec_work_yy", seenAuth)
	}
}

// TestDoctor_QuietMode_PrintsScalar pins Round 8 JJ: doctor's quiet
// mode used to dump the full JSON tree, violating the "quiet = single
// useful scalar" contract. Now prints the overall status ("ok" or
// "fail") on one line.
func TestDoctor_QuietMode_PrintsScalar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	_ = cmd.Execute([]string{"doctor", "--output-format", "quiet"}, &stdout, &stderr)
	out := strings.TrimSpace(stdout.String())
	if strings.HasPrefix(out, "{") || strings.Contains(out, "\"checks\"") {
		t.Errorf("quiet mode emitted JSON tree: %s", out)
	}
	if out != "ok" && out != "fail" {
		t.Errorf("quiet mode should print 'ok' or 'fail' only; got %q", out)
	}
}

// TestDoctor_ExitCode_AuthFail pins Round 8 JJ: when only credential
// checks fail (no api_secret), exit code should be 3 (auth), not 10
// (server). The contract maps exit 10 to upstream-server problems,
// which is misleading when the actual issue is local config.
func TestDoctor_ExitCode_AuthFail(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"doctor", "--output-format", "json"}, &stdout, &stderr)
	// We can't be 100% sure DNS will succeed in CI, so accept either
	// auth-only (exit 3) or network+auth (exit 11 — network wins by
	// priority). 10 (server) without an upstream problem is wrong.
	if exit != 3 && exit != 11 {
		t.Errorf("exit=%d, want 3 (auth) or 11 (network) — not 10", exit)
	}
}

// TestDoctor_OverallStatusField pins that the new data.status scalar
// is present in JSON output (agents can read this without iterating
// data.checks).
func TestDoctor_OverallStatusField(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	_ = cmd.Execute([]string{"doctor", "--output-format", "json"}, &stdout, &stderr)
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data, _ := env["data"].(map[string]any)
	status, _ := data["status"].(string)
	if status != "ok" && status != "fail" {
		t.Errorf("data.status=%q, want 'ok' or 'fail'", status)
	}
}
