package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func writeConfig(t *testing.T, dir, body string) {
	t.Helper()
	cfgDir := filepath.Join(dir, "urlbox")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func loginSubcommand(t *testing.T) *cobra.Command {
	t.Helper()
	var out, errOut bytes.Buffer
	root := newRootCmd(&out, &errOut)
	for _, c := range root.Commands() {
		if c.Name() == "login" {
			return c
		}
	}
	t.Fatal("login subcommand not registered on root")
	return nil
}

func advanceClockUntil(t *testing.T, fc *clock.FakeClock, done <-chan struct{}) {
	t.Helper()
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				if fc.WaitForSleeper(5 * time.Millisecond) {
					fc.Advance(10 * time.Second)
				}
			}
		}
	}()
}

func TestLoginFullFlowSingleOrgSingleProject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"device_code":"dev_1","user_code":"ABCD-1234","verification_uri_complete":"https://urlbox.com/device?code=ABCD-1234","interval":5,"expires_in":300}`),
		apitest.SuccessJSON(`{"access_token":"sess_tok_new"}`),
		apitest.SuccessJSON(`[{"id":"7","name":"Acme","publicId":"org_acme"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_fetched","apiSecret":"sk_fetched","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	SetLoginClockForTest(fc)
	t.Cleanup(ResetLoginClockForTest)
	done := make(chan struct{})
	defer close(done)
	advanceClockUntil(t, fc, done)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"login", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Email string `json:"email"`
			Org   struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"org"`
			Project *struct {
				ID string `json:"id"`
			} `json:"project"`
			Render struct {
				Credential string `json:"credential"`
			} `json:"render"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not an envelope: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Data.Email != "a@urlbox.com" || env.Data.Org.ID != "org_acme" {
		t.Fatalf("envelope: %s", stdout.String())
	}
	if env.Data.Project == nil || env.Data.Project.ID != "proj_1" {
		t.Fatalf("project: %s", stdout.String())
	}
	if env.Data.Render.Credential != "ready" {
		t.Fatalf("render status: %s", stdout.String())
	}

	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Profiles map[string]map[string]string `json:"profiles"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	p := cfg.Profiles["default"]
	if p["session_token"] != "sess_tok_new" || p["active_org"] != "org_acme" ||
		p["active_project"] != "proj_1" || p["api_secret"] != "sk_fetched" || p["api_key"] != "pk_fetched" {
		t.Fatalf("profile after login: %#v", p)
	}

	reqs := srv.Requests()
	if reqs[0].Path != "/v1/auth/device/code" {
		t.Fatalf("first call %q", reqs[0].Path)
	}
	if !bytes.Contains(reqs[0].Body, []byte(`"client_id":"urlbox-cli"`)) {
		t.Fatalf("device/code body: %s", reqs[0].Body)
	}
	if reqs[1].Path != "/v1/auth/device/token" {
		t.Fatalf("second call %q", reqs[1].Path)
	}
	if got := reqs[2].Header.Get("Authorization"); got != "Bearer sess_tok_new" {
		t.Fatalf("org list auth header %q", got)
	}
	if stderrStr := stderr.String(); !bytes.Contains([]byte(stderrStr), []byte("ABCD-1234")) {
		t.Fatalf("user code must print to stderr, got: %s", stderrStr)
	}
}

type recordingOpener struct{ opened []string }

func (r *recordingOpener) Open(url string) error {
	r.opened = append(r.opened, url)
	return nil
}

func TestLoginQuietPrintsEmailScalar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"device_code":"dev_1","user_code":"ABCD-1234","verification_uri_complete":"https://urlbox.com/device?code=ABCD-1234","interval":5,"expires_in":300}`),
		apitest.SuccessJSON(`{"access_token":"sess_tok_new"}`),
		apitest.SuccessJSON(`[{"id":"7","name":"Acme","publicId":"org_acme"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_fetched","apiSecret":"sk_fetched","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	SetLoginClockForTest(fc)
	t.Cleanup(ResetLoginClockForTest)
	done := make(chan struct{})
	defer close(done)
	advanceClockUntil(t, fc, done)

	rec := &recordingOpener{}
	SetLoginOpenerForTest(rec)
	t.Cleanup(ResetLoginOpenerForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"login", "--output-format", "quiet"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if out != `"a@urlbox.com"` {
		t.Fatalf("quiet stdout should be the bare email scalar; got %q", out)
	}
	if len(rec.opened) != 0 {
		t.Fatalf("quiet mode must not open the browser; opened %v", rec.opened)
	}
}

func TestLoginPersistsKeyAndSecretFromSameCredential(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"device_code":"dev_1","user_code":"ABCD-1234","verification_uri_complete":"https://urlbox.com/device?code=ABCD-1234","interval":5,"expires_in":300}`),
		apitest.SuccessJSON(`{"access_token":"sess_tok_new"}`),
		apitest.SuccessJSON(`[{"id":"7","name":"Acme","publicId":"org_acme"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_revoked","apiSecret":"sk_revoked","revoked":true},{"apiKey":"pk_live","apiSecret":"sk_live","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	SetLoginClockForTest(fc)
	t.Cleanup(ResetLoginClockForTest)
	done := make(chan struct{})
	defer close(done)
	advanceClockUntil(t, fc, done)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"login", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}

	b, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg struct {
		Profiles map[string]map[string]string `json:"profiles"`
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	p := cfg.Profiles["default"]
	if p["api_key"] != "pk_live" || p["api_secret"] != "sk_live" {
		t.Fatalf("key and secret must come from the same non-revoked credential: %#v", p)
	}
}

func TestLoginDeniedExitsAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	srv := apitest.New(
		apitest.SuccessJSON(`{"device_code":"dev_1","user_code":"ABCD-1234","verification_uri_complete":"https://urlbox.com/device","interval":5,"expires_in":300}`),
		apitest.ScriptedResponse{Status: 400, Body: `{"error":"access_denied"}`},
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	SetLoginClockForTest(fc)
	t.Cleanup(ResetLoginClockForTest)
	done := make(chan struct{})
	defer close(done)
	advanceClockUntil(t, fc, done)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"login", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (auth)\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not an envelope: %v\n%s", err, stdout.String())
	}
	if env["code"] != "auth" {
		t.Fatalf("error envelope code = %v, want auth: %s", env["code"], stdout.String())
	}
}

func TestLoadSessionNoTokenReturnsAuth(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, `{"default_profile":"default","profiles":{"default":{"api_host":"https://api.urlbox.com"}}}`)

	_, cliErr := loadSession(loginSubcommand(t))
	if cliErr == nil {
		t.Fatal("expected an auth error when the profile has no session token")
	}
	if cliErr.Code != output.ErrAuth {
		t.Fatalf("code = %q, want auth", cliErr.Code)
	}
	if cliErr.Hint == "" {
		t.Fatal("hint must be non-empty")
	}
}

func TestLoadSessionWithTokenBuildsClient(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	writeConfig(t, dir, `{"default_profile":"default","profiles":{"default":{"api_host":"https://api.urlbox.com","session_token":"sess_tok"}}}`)

	state, cliErr := loadSession(loginSubcommand(t))
	if cliErr != nil {
		t.Fatalf("unexpected error: %v", cliErr)
	}
	if state.ProfileName != "default" {
		t.Fatalf("profile name = %q, want default", state.ProfileName)
	}
	if state.Profile.SessionToken != "sess_tok" {
		t.Fatalf("session token = %q, want sess_tok", state.Profile.SessionToken)
	}
	if state.Client == nil {
		t.Fatal("client must be constructed")
	}
	if state.Host != "https://api.urlbox.com" {
		t.Fatalf("host = %q, want https://api.urlbox.com", state.Host)
	}
}
