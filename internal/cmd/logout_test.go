package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func readProfileMap(t *testing.T, dir string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Profiles map[string]map[string]string `json:"profiles"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg.Profiles["default"]
}

func TestLogoutRevokesAndClears(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Path != "/v1/auth/sign-out" {
		t.Fatalf("requests: %+v", reqs)
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer sess_tok_compat_123456" {
		t.Fatalf("auth header %q", got)
	}
	p := readProfileMap(t, dir)
	for _, key := range []string{"session_token", "active_org", "active_project", "api_key", "api_secret"} {
		if p[key] != "" {
			t.Fatalf("%s not cleared: %#v", key, p)
		}
	}
}

func TestLogoutOfflineStillClearsLocally(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_HOST", "http://127.0.0.1:1")

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"logout", "--no-retry", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("offline logout must still succeed, exit %d\n%s", code, stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("clearing local login anyway")) {
		t.Fatalf("expected warning on stderr, got: %s", stderr.String())
	}
	if p := readProfileMap(t, dir); p["session_token"] != "" {
		t.Fatalf("token not cleared: %#v", p)
	}
}

func TestLogoutWhenNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"logout", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("logout without a session must be a no-op success, exit %d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok": true`)) {
		t.Fatalf("envelope: %s", stdout.String())
	}
}
