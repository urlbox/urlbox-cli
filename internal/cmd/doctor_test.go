package cmd_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
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
	for _, name := range []string{"version", "install_method", "config_file", "api_key", "dns", "api_reachable", "auth"} {
		_ = extractCheck(t, env, name)
	}
}

func TestDoctor_NoAPIKey_FailsAPIKeyCheck(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	env, exit, _, _ := runDoctor(t)

	c := extractCheck(t, env, "api_key")
	if c["status"] != "fail" {
		t.Fatalf("api_key status = %v want fail", c["status"])
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
