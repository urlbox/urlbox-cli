package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
	"github.com/urlbox/urlbox-cli/internal/clock"
)

// TestLoginThenRenderDryRun is the spec-promised login → render sequence: a
// scripted device login writes a real profile (session token + render secret),
// then `render --dry-run` reads that same profile and validates the payload
// without touching the network. It exercises the compatibility promise that
// login writes into the same file render reads.
func TestLoginThenRenderDryRun(t *testing.T) {
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

	var loginOut, loginErr bytes.Buffer
	if code := Execute([]string{"login", "--output-format", "json"}, &loginOut, &loginErr); code != 0 {
		t.Fatalf("login exit %d\n%s\n%s", code, loginOut.String(), loginErr.String())
	}

	var renderOut, renderErr bytes.Buffer
	code := Execute([]string{"render", "https://example.com", "--dry-run", "--output-format", "json"}, &renderOut, &renderErr)
	if code != 0 {
		t.Fatalf("render dry-run exit %d\n%s\n%s", code, renderOut.String(), renderErr.String())
	}

	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(renderOut.Bytes(), &env); err != nil {
		t.Fatalf("render dry-run stdout not an envelope: %v\n%s", err, renderOut.String())
	}
	if !env.OK {
		t.Fatalf("render dry-run should succeed against the logged-in profile: %s", renderOut.String())
	}
}
