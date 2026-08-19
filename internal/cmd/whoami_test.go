package cmd

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestWhoamiNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("exit = %d, want 3 (auth)", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"code": "auth"`)) ||
		!bytes.Contains(stdout.Bytes(), []byte("urlbox login")) {
		t.Fatalf("error envelope must carry auth code + login hint: %s", stdout.String())
	}
}

func TestWhoamiHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Main"}]}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"7","activeOrganizationPublicId":"org_acme"}}`),
		apitest.SuccessJSON(`[{"id":"7","name":"Acme","publicId":"org_acme"}]`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	var env struct {
		Data struct {
			Email string `json:"email"`
			Org   struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"org"`
			Project struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"project"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.Data.Email != "a@urlbox.com" || env.Data.Org.ID != "org_acme" ||
		env.Data.Org.Name != "Acme" || env.Data.Project.ID != "proj_compat" {
		t.Fatalf("data: %s", stdout.String())
	}
}

func TestWhoamiExpiredSessionIsAuthError(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"user":{"email":""},"session":{"activeOrganizationId":"","activeOrganizationPublicId":""}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"whoami", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("dead token must exit 3, got %d\n%s", code, stdout.String())
	}
}

func TestMeAliasWorks(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, false)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"me", "--output-format", "json"}, &stdout, &stderr)
	if code != 3 {
		t.Fatalf("me alias must route to whoami, exit %d", code)
	}
}
