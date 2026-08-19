package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

// Switching orgs deliberately drops the stored render credential: it belongs to
// a project in the org being left, and keeping it would let `urlbox render`
// bill the previous organisation silently. These tests pin the two things that
// must be true *around* that drop — it clears the whole credential pair, and it
// never reports an incomplete context as an unqualified success.

const twoOrgsJSON = `[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`

const sessionOrgOneJSON = `{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"1","activeOrganizationPublicId":"org_one"}}`

func TestOrgsSelectAmbiguousProjectsClearsWholeCredentialPair(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(twoOrgsJSON),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(sessionOrgOneJSON),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"alpha"},{"id":"proj_b","name":"beta"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	Execute([]string{"orgs", "select", "one", "--output-format", "json"}, &stdout, &stderr)
	p := readProfileMap(t, dir)
	if p["api_secret"] != "" {
		t.Fatalf("api_secret must be cleared on an org switch: %#v", p)
	}
	if p["api_key"] != "" {
		t.Fatalf("api_key must be cleared with the secret — a stale key from the previous org survived: %#v", p)
	}
	if p["active_project"] != "" {
		t.Fatalf("active_project must be cleared: %#v", p)
	}
}

func TestOrgsSelectAmbiguousProjectsReportsIncompleteContext(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(twoOrgsJSON),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(sessionOrgOneJSON),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"alpha"},{"id":"proj_b","name":"beta"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	Execute([]string{"orgs", "select", "one", "--output-format", "json"}, &stdout, &stderr)
	// The next command must be reachable from the envelope alone — an agent
	// reading stdout never sees the stderr line.
	if !bytes.Contains(stdout.Bytes(), []byte("urlbox projects select")) {
		t.Fatalf("envelope must breadcrumb to `urlbox projects select`: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("2 projects")) {
		t.Fatalf("summary must say the context is unfinished: %s", stdout.String())
	}
}

func TestOrgsSelectProjectFlagCompletesInOneStep(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(twoOrgsJSON),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(sessionOrgOneJSON),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"alpha"},{"id":"proj_b","name":"beta"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_beta","apiSecret":"sk_beta","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "one", "--project", "beta", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	p := readProfileMap(t, dir)
	if p["active_org"] != "org_one" || p["active_project"] != "proj_b" ||
		p["api_key"] != "pk_beta" || p["api_secret"] != "sk_beta" {
		t.Fatalf("--project must land a complete context in one call: %#v", p)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"credential": "ready"`)) {
		t.Fatalf("render credential should be ready: %s", stdout.String())
	}
}

func TestOrgsSelectProjectFlagUnknownProjectErrors(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(twoOrgsJSON),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(sessionOrgOneJSON),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"alpha"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"orgs", "select", "one", "--project", "nope", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("an unknown --project must not exit 0: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"not_found"`)) {
		t.Fatalf("expected a not_found envelope: %s", stdout.String())
	}
}
