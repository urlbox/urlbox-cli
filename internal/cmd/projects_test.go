package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func TestProjectsListMarksActive(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Main"},{"id":"proj_2","name":"Side"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"proj_compat"`, `"proj_2"`, `"active": true`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("missing %s: %s", want, stdout.String())
		}
	}
}

func TestProjectsSelectPositionalRefreshesCredential(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Main"},{"id":"proj_2","name":"Side"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_side","apiSecret":"sk_side","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "select", "side", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_2" || p["api_secret"] != "sk_side" || p["api_key"] != "pk_side" {
		t.Fatalf("profile after select: %#v", p)
	}
}

func TestProjectsCreateSelectPersistsAndRefreshesCredential(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"id":"proj_new","name":"Fresh"}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_fresh","apiSecret":"sk_fresh","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "create", "Fresh", "--select", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}

	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_new" || p["api_key"] != "pk_fresh" || p["api_secret"] != "sk_fresh" {
		t.Fatalf("profile after create --select: %#v", p)
	}

	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("create --select should POST the project then fetch the credential, got %d requests: %+v", len(reqs), reqs)
	}
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/projects" {
		t.Fatalf("first request must create the project: %+v", reqs[0])
	}
	if reqs[1].Method != "GET" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_new/api-credentials" {
		t.Fatalf("second request must fetch the new project's render credential: %+v", reqs[1])
	}

	if !bytes.Contains(stdout.Bytes(), []byte(`"selected": true`)) {
		t.Fatalf("envelope must note the switch with selected:true:\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("now active")) {
		t.Fatalf("summary must reflect the switch:\n%s", stdout.String())
	}
}

func TestProjectsCreateNonTTYWithoutSelectMakesNoExtraCalls(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"proj_new","name":"Fresh"}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "create", "Fresh", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}

	if reqs := srv.Requests(); len(reqs) != 1 {
		t.Fatalf("create without --select off-TTY must only POST the project, got %d: %+v", len(reqs), reqs)
	}
	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_compat" {
		t.Fatalf("active project must be untouched, got %q", p["active_project"])
	}
	if bytes.Contains(stdout.Bytes(), []byte(`"selected": true`)) {
		t.Fatalf("no switch happened, selected:true must not appear:\n%s", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("now active")) {
		t.Fatalf("summary must not claim a switch:\n%s", stdout.String())
	}
}

func TestProjectsCreateDeclinedConfirmSkipsSwitchToleratesWrappedResponse(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"project":{"id":"proj_new","name":"Fresh"}}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetConfirmPromptForTest(func(string) (bool, error) { return false, nil })
	t.Cleanup(ResetConfirmPromptForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "create", "Fresh", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}

	if reqs := srv.Requests(); len(reqs) != 1 {
		t.Fatalf("declining the switch must make no extra calls, got %d: %+v", len(reqs), reqs)
	}
	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_compat" {
		t.Fatalf("declined switch must leave the active project untouched, got %q", p["active_project"])
	}
	if strings.Contains(stdout.String(), "now active") {
		t.Fatalf("declined switch must not claim the project is active:\n%s", stdout.String())
	}
}

func TestProjectsCreateAcceptedConfirmSwitches(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"id":"proj_new","name":"Fresh"}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_fresh","apiSecret":"sk_fresh","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetConfirmPromptForTest(func(string) (bool, error) { return true, nil })
	t.Cleanup(ResetConfirmPromptForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "create", "Fresh", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}

	p := readProfileMap(t, dir)
	if p["active_project"] != "proj_new" || p["api_secret"] != "sk_fresh" {
		t.Fatalf("accepted switch must activate the new project: %#v", p)
	}
	if !strings.Contains(stdout.String(), "now active") {
		t.Fatalf("accepted switch summary must say now active:\n%s", stdout.String())
	}
}

func TestProjectsSelectNonInteractiveWithoutArg(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"A"},{"id":"proj_2","name":"B"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "select", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("want usage exit 1, got %d\n%s", code, stdout.String())
	}
}
