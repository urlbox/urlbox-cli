package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const projectAssignedJSON = `{"id":"proj_1","name":"Main","enabled":true,"storageCredentialId":"store_1","proxyId":"pool_1","llmCredentialId":"llm_1","createdAt":"2026-08-01T00:00:00.000Z"}`

const projectUnassignedJSON = `{"id":"proj_1","name":"Main","enabled":true,"storageCredentialId":null,"proxyId":null,"llmCredentialId":null,"createdAt":"2026-08-01T00:00:00.000Z"}`

const projectListJSON = `{"projects":[{"id":"proj_1","name":"Main"}]}`

func TestProjectsStorageAssignResolvesBothAndPuts(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(storageListJSON),
		apitest.SuccessJSON(projectAssignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "storage", "assign", "main", "prod", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	put := reqs[len(reqs)-1]
	if put.Method != "PUT" || put.Path != "/v2/organisation/org_compat/projects/proj_1/storage-credential" {
		t.Fatalf("assign request: %+v", put)
	}
	if !bytes.Contains(put.Body, []byte(`"storageCredentialId":"store_1"`)) {
		t.Fatalf("assign body missing storageCredentialId: %s", put.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Assigned prod to Main")) {
		t.Fatalf("summary must name both credential and project: %s", stdout.String())
	}
}

func TestProjectsProxyAssignResolvesBothAndPuts(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(proxyListJSON),
		apitest.SuccessJSON(projectAssignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "proxy", "assign", "main", "eu", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	put := reqs[len(reqs)-1]
	if put.Method != "PUT" || put.Path != "/v2/organisation/org_compat/projects/proj_1/proxy" {
		t.Fatalf("assign request: %+v", put)
	}
	if !bytes.Contains(put.Body, []byte(`"proxyId":"pool_1"`)) {
		t.Fatalf("assign body missing proxyId: %s", put.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Assigned eu to Main")) {
		t.Fatalf("summary must name both pool and project: %s", stdout.String())
	}
}

func TestProjectsLlmAssignResolvesBothAndPuts(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(llmListJSON),
		apitest.SuccessJSON(projectAssignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "llm", "assign", "main", "openai-prod", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	put := reqs[len(reqs)-1]
	if put.Method != "PUT" || put.Path != "/v2/organisation/org_compat/projects/proj_1/llm-credential" {
		t.Fatalf("assign request: %+v", put)
	}
	if !bytes.Contains(put.Body, []byte(`"llmCredentialId":"llm_1"`)) {
		t.Fatalf("assign body missing llmCredentialId: %s", put.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Assigned openai-prod to Main")) {
		t.Fatalf("summary must name both credential and project: %s", stdout.String())
	}
}

func TestProjectsStorageUnassignDeletes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(projectUnassignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "storage", "unassign", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	del := reqs[len(reqs)-1]
	if del.Method != "DELETE" || del.Path != "/v2/organisation/org_compat/projects/proj_1/storage-credential" {
		t.Fatalf("unassign request: %+v", del)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Unassigned the storage credential from Main")) {
		t.Fatalf("summary must name the noun and project: %s", stdout.String())
	}
}

func TestProjectsProxyUnassignDeletes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(projectUnassignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "proxy", "unassign", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	del := reqs[len(reqs)-1]
	if del.Method != "DELETE" || del.Path != "/v2/organisation/org_compat/projects/proj_1/proxy" {
		t.Fatalf("unassign request: %+v", del)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Unassigned the proxy pool from Main")) {
		t.Fatalf("summary must name the noun and project: %s", stdout.String())
	}
}

func TestProjectsLlmUnassignDeletes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(projectUnassignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "llm", "unassign", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	del := reqs[len(reqs)-1]
	if del.Method != "DELETE" || del.Path != "/v2/organisation/org_compat/projects/proj_1/llm-credential" {
		t.Fatalf("unassign request: %+v", del)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Unassigned the LLM credential from Main")) {
		t.Fatalf("summary must name the noun and project: %s", stdout.String())
	}
}

func TestProjectsAssignJSONCarriesReturnedProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(storageListJSON),
		apitest.SuccessJSON(projectAssignedJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "storage", "assign", "proj_1", "store_1", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"storageCredentialId": "store_1"`, `"id": "proj_1"`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("json envelope must carry the returned project, missing %s: %s", want, stdout.String())
		}
	}
}

func TestProjectsAssignUnknownProjectHint(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(projectListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "storage", "assign", "nope", "store_1", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("assign to an unknown project must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("urlbox projects list")) {
		t.Fatalf("not-found hint must name `urlbox projects list`: %s", stdout.String())
	}
	for _, r := range srv.Requests() {
		if r.Method == "PUT" {
			t.Fatalf("no PUT must be issued when the project is unknown: %+v", r)
		}
	}
}

func TestProjectsAssignUnknownCredHint(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(projectListJSON),
		apitest.SuccessJSON(storageListJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "storage", "assign", "main", "nope", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("assign of an unknown credential must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("urlbox storage list")) {
		t.Fatalf("not-found hint must name `urlbox storage list`: %s", stdout.String())
	}
	for _, r := range srv.Requests() {
		if r.Method == "PUT" {
			t.Fatalf("no PUT must be issued when the credential is unknown: %+v", r)
		}
	}
}
