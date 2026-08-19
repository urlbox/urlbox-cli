package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const proxyListJSON = `{"proxies":[
 {"id":"pool_1","name":"eu","proxies":[{"id":"proxy_1","name":"one","url":"http://user:hunter2@p1.example.com:8080"},{"id":"proxy_2","name":"","url":"http://p2.example.com:8080"}],"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"}]}`

const proxyOneJSON = `{"id":"pool_1","name":"eu","proxies":[{"id":"proxy_1","name":"one","url":"http://user:hunter2@p1.example.com:8080"},{"id":"proxy_2","name":"","url":"http://p2.example.com:8080"}],"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"}`

func TestProxiesListShowsCountNeverURLs(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(proxyListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "list", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "GET" || reqs[0].Path != "/v2/organisation/org_compat/proxies" {
		t.Fatalf("request: %+v", reqs[0])
	}
	out := stdout.String()
	for _, want := range []string{"pool_1", "eu", "URLS", "ASSIGNED"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("list output missing %q: %s", want, out)
		}
	}
	for _, leak := range []string{"hunter2", "p1.example.com", "p2.example.com"} {
		if bytes.Contains(stdout.Bytes(), []byte(leak)) {
			t.Fatalf("list must never print URLs, leaked %q: %s", leak, out)
		}
	}
}

func TestProxiesShowMasksPasswordOnly(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(proxyListJSON),
		apitest.SuccessJSON(proxyOneJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "show", "pool_1", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "GET" || reqs[1].Path != "/v2/organisation/org_compat/proxies/pool_1" {
		t.Fatalf("show request: %+v", reqs[1])
	}
	out := stdout.String()
	if !bytes.Contains(stdout.Bytes(), []byte("http://user:****@p1.example.com:8080")) {
		t.Fatalf("show must mask the password portion only: %s", out)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("http://p2.example.com:8080")) {
		t.Fatalf("passwordless URL must be shown untouched: %s", out)
	}
	if bytes.Contains(stdout.Bytes(), []byte("hunter2")) {
		t.Fatalf("show must not leak the password without --reveal: %s", out)
	}

	srv2 := apitest.New(
		apitest.SuccessJSON(proxyListJSON),
		apitest.SuccessJSON(proxyOneJSON),
	)
	t.Cleanup(srv2.Close)
	t.Setenv("URLBOX_API_HOST", srv2.URL())
	var revealOut, revealErr bytes.Buffer
	code = Execute([]string{"proxies", "show", "pool_1", "--reveal", "--output-format", "text"}, &revealOut, &revealErr)
	if code != 0 {
		t.Fatalf("reveal exit %d\n%s\n%s", code, revealOut.String(), revealErr.String())
	}
	if !bytes.Contains(revealOut.Bytes(), []byte("hunter2")) {
		t.Fatalf("--reveal must show the full password: %s", revealOut.String())
	}
}

func TestProxiesCreateBody(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"pool_new","name":"eu","proxies":[{"id":"proxy_a","name":"","url":"http://a:1"}],"assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"proxies", "create",
		"--name", "eu", "--url", "http://a:1", "--url", "http://b:2",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/proxies" {
		t.Fatalf("create request: %+v", reqs[0])
	}
	if string(reqs[0].Body) != `{"name":"eu","proxies":[{"url":"http://a:1"},{"url":"http://b:2"}]}` {
		t.Fatalf("create body wrong: %s", reqs[0].Body)
	}
}

func TestProxiesCreatePositionalName(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"pool_new","name":"eu","proxies":[{"id":"proxy_a","name":"","url":"http://a:1"}],"assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"proxies", "create", "eu",
		"--url", "http://a:1", "--url", "http://b:2",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/proxies" {
		t.Fatalf("create request: %+v", reqs[0])
	}
	if string(reqs[0].Body) != `{"name":"eu","proxies":[{"url":"http://a:1"},{"url":"http://b:2"}]}` {
		t.Fatalf("create body must carry the positional name: %s", reqs[0].Body)
	}
}

func TestProxiesCreatePositionalConflictsWithFlag(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"proxies", "create", "a", "--name", "b", "--url", "http://a:1",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("conflicting name must fail\n%s", stdout.String())
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("conflict must make no API call: %+v", srv.Requests())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("--name")) {
		t.Fatalf("conflict error must name the flag: %s", stdout.String())
	}
}

func TestProxiesUpdateReplacesWholeList(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(proxyOneJSON),
		apitest.SuccessJSON(`{"id":"pool_1","name":"eu"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"proxies", "update", "pool_1",
		"--url", "http://new:1", "--url", "http://new:2",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "GET" || reqs[0].Path != "/v2/organisation/org_compat/proxies/pool_1" {
		t.Fatalf("update must fetch the existing pool first: %+v", reqs[0])
	}
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/proxies/pool_1" {
		t.Fatalf("update request: %+v", reqs[1])
	}
	if string(reqs[1].Body) != `{"name":"eu","proxies":[{"url":"http://new:1"},{"url":"http://new:2"}]}` {
		t.Fatalf("update must replace the whole list with exactly the flags sent: %s", reqs[1].Body)
	}
}

func TestProxiesUpdateNameOnlyCarriesListForward(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(proxyOneJSON),
		apitest.SuccessJSON(`{"id":"pool_1","name":"west"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"proxies", "update", "pool_1", "--name", "west", "--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "GET" || reqs[0].Path != "/v2/organisation/org_compat/proxies/pool_1" {
		t.Fatalf("update must fetch the existing pool first: %+v", reqs[0])
	}
	if reqs[1].Method != "PATCH" {
		t.Fatalf("update request: %+v", reqs[1])
	}
	want := `{"name":"west","proxies":[{"name":"one","url":"http://user:hunter2@p1.example.com:8080"},{"url":"http://p2.example.com:8080"}]}`
	if string(reqs[1].Body) != want {
		t.Fatalf("name-only update must carry the existing list forward:\n got %s\nwant %s", reqs[1].Body, want)
	}
}

func TestProxiesUpdateHelpWarnsWholeListReplacement(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "update", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	want := "The server replaces the pool's proxy list with exactly what you send: passing any --url replaces the whole list; omitting --url keeps the existing list."
	if !bytes.Contains(stdout.Bytes(), []byte(want)) {
		t.Fatalf("update --help must state the whole-list-replacement rule: %s", stdout.String())
	}
}

func TestProxiesDeleteRequiresYesOffTTY(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(proxyListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "delete", "pool_1", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("delete without --yes off-TTY must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("--yes")) {
		t.Fatalf("error must name --yes: %s", stdout.String())
	}
	for _, r := range srv.Requests() {
		if r.Method == "DELETE" {
			t.Fatalf("no DELETE must be issued without confirmation: %+v", r)
		}
	}
}

func TestProxiesDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(proxyListJSON),
		apitest.SuccessJSON(`{"id":"pool_1","deleted":true}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "delete", "pool_1", "--yes", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Method != "DELETE" || last.Path != "/v2/organisation/org_compat/proxies/pool_1" {
		t.Fatalf("delete request: %+v", last)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("eu")) {
		t.Fatalf("delete summary must name the pool: %s", stdout.String())
	}
}

func TestProxiesCreateAssignTo(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"id":"pool_new","name":"eu","proxies":[{"id":"proxy_a","url":"http://a:1"}],"assignedProjectIds":[]}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"id":"proj_1","name":"Main","proxyId":"pool_new"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"proxies", "create",
		"--name", "eu", "--url", "http://a:1",
		"--assign-to", "proj_1",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	var put *apitest.CapturedRequest
	for i := range reqs {
		if reqs[i].Method == "PUT" {
			put = &reqs[i]
		}
	}
	if put == nil {
		t.Fatalf("assign must issue a PUT, requests: %+v", reqs)
	}
	if put.Path != "/v2/organisation/org_compat/projects/proj_1/proxy" {
		t.Fatalf("assign PUT path: %s", put.Path)
	}
	if !bytes.Contains(put.Body, []byte(`"proxyId":"pool_new"`)) {
		t.Fatalf("assign body missing proxyId: %s", put.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"assigned"`)) {
		t.Fatalf("envelope data must carry the assigned project: %s", stdout.String())
	}
}

func TestProxiesNotFoundNameHint(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(proxyListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "show", "nope", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("show of an unknown name must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("urlbox proxies list")) {
		t.Fatalf("not-found hint must name `urlbox proxies list`: %s", stdout.String())
	}
}
