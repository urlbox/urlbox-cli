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

func TestDoctor_AllChecksPass_Exit0(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)
	if exit != 0 {
		t.Fatalf("exit=%d env=%v", exit, env)
	}
	if env["ok"] != true {
		t.Fatalf("ok=%v", env["ok"])
	}

	// Validate at least these checks are present
	for _, name := range []string{"version", "install_method", "config_file", "api_secret", "dns", "api_reachable", "auth"} {
		_ = extractCheck(t, env, name)
	}
}

func TestDoctor_NoAPISecret_FailsAPISecretCheck(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "api_secret")
	if c["status"] != "fail" {
		t.Fatalf("api_secret status = %v want fail", c["status"])
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when checks fail")
	}
}

func TestDoctor_AuthFailure_FailsAuthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_bad")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "auth")
	if c["status"] != "fail" {
		t.Fatalf("auth check status = %v want fail", c["status"])
	}
	if !strings.Contains(c["message"].(string), "credentials") &&
		!strings.Contains(c["hint"].(string), "auth") {
		t.Fatalf("auth check message should reference credentials: %v", c)
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit on auth failure")
	}
}

// TestDoctor_AuthBadRequest_FailsAuthCheck pins Round 4 H2: the auth check
// previously only treated 401/403/5xx as failure. A real-world 400 from
// /v1/user/me with body {"error":{"code":"ApiKeyNotFound",...}} fell into
// the default arm and was reported as "credentials valid" — a critical
// false-positive that let CI green-light a broken secret.
func TestDoctor_AuthBadRequest_FailsAuthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"ApiKeyNotFound","message":"Api Key does not exist"}}`))
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "ubx_sk_fakebutwellformed")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "auth")
	if c["status"] != "fail" {
		t.Fatalf("auth check on HTTP 400 should fail; got %v (full check: %v)", c["status"], c)
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when auth check fails")
	}
}

// TestDoctor_AuthNotFound_FailsAuthCheck pins another non-2xx, non-401/403
// case — 404 on /v1/user/me should not be "credentials valid" either.
func TestDoctor_AuthNotFound_FailsAuthCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "ubx_sk_anything")
	t.Setenv("URLBOX_API_HOST", srv.URL)

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "auth")
	if c["status"] != "fail" {
		t.Fatalf("auth check on HTTP 404 should fail; got %v", c["status"])
	}
	if exit == 0 {
		t.Fatal("expected non-zero exit when auth check fails")
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
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_SECRET", "") // force file lookup
	t.Setenv("URLBOX_API_HOST", srv.URL)
	seedConfig(t, dir, map[string]config.Profile{
		"default": {APISecret: "sec_default_xx"},
		"work":    {APISecret: "sec_work_yy"},
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
