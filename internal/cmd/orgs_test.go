package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestOrgsListMarksActive(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"2","activeOrganizationPublicId":"org_two"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"org_one"`, `"org_two"`, `"active": true`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("missing %s in: %s", want, stdout.String())
		}
	}
}

func TestOrgsSelectPositionalSwitchesAndRefreshesProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"1","activeOrganizationPublicId":"org_one"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_9","name":"OtherOrgProj"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_other_org","apiSecret":"sk_other_org","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "one", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	p := readProfileMap(t, dir)
	if p["active_org"] != "org_one" || p["active_project"] != "proj_9" ||
		p["api_secret"] != "sk_other_org" || p["api_key"] != "pk_other_org" {
		t.Fatalf("profile after select: %#v", p)
	}
	reqs := srv.Requests()
	if reqs[1].Path != "/v1/auth/organization/set-active" {
		t.Fatalf("second call %q", reqs[1].Path)
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"organizationId":"1"`)) {
		t.Fatalf("set-active body: %s", reqs[1].Body)
	}
}

func TestOrgsSelectInteractiveProjectPicker(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"1","activeOrganizationPublicId":"org_one"}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"Alpha"},{"id":"proj_b","name":"Beta"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_beta","apiSecret":"sk_beta","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	picked := false
	SetOrgsProjectPickForTest(func(_ string, options []string, _ int) (int, error) {
		picked = true
		if len(options) != 2 {
			t.Fatalf("picker options = %v, want 2", options)
		}
		return 1, nil
	})
	t.Cleanup(ResetOrgsProjectPickForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "one", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if !picked {
		t.Fatal("interactive project picker was not invoked")
	}
	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_b" || p["api_secret"] != "sk_beta" || p["api_key"] != "pk_beta" {
		t.Fatalf("picked project must be persisted: %#v", p)
	}
}

func TestOrgsSelectProjectStepServerErrorReportsNoActiveProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"1","activeOrganizationPublicId":"org_one"}}`),
		apitest.ServerError(500),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "one", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("org switch must still exit 0 on a project-step error, got %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("no active project set")) {
		t.Fatalf("stderr must report the project-step failure: %s", stderr.String())
	}
	if bytes.Contains(stderr.Bytes(), []byte("Several projects")) {
		t.Fatalf("a 500 must not be reported as several projects: %s", stderr.String())
	}
}

func TestOrgsSelectNonInteractiveWithoutArg(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("want usage exit 1, got %d\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("name-or-id")) {
		t.Fatalf("error must name the positional: %s", stdout.String())
	}
}
