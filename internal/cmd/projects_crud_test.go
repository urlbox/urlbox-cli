package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func writeCompatConfigNoOrg(t *testing.T, dir string) {
	t.Helper()
	cfg := map[string]any{
		"default_profile": "default",
		"profiles": map[string]any{
			"default": map[string]string{
				"api_key":        "pk_test_key",
				"api_secret":     compatSecret,
				"session_token":  "sess_tok_compat_123456",
				"active_project": "proj_compat",
			},
		},
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "urlbox"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "urlbox", "config.json"), b, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestProjectsCreate(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"project":{"id":"proj_new","name":"Fresh"}}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "create", "Fresh", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/projects" {
		t.Fatalf("request: %+v", reqs[0])
	}
	if !bytes.Contains(reqs[0].Body, []byte(`"name":"Fresh"`)) {
		t.Fatalf("body: %s", reqs[0].Body)
	}
}

func TestProjectsRename(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Old"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","name":"New"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "rename", "old", "New", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"name":"New"`)) {
		t.Fatalf("body: %s", reqs[1].Body)
	}
}

func TestProjectsDeleteRequiresYesOffTTY(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Doomed"}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("delete without --yes off-TTY must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("--yes")) {
		t.Fatalf("error must name --yes: %s", stdout.String())
	}
}

func TestProjectsDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "DELETE" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
}

func readProfile(t *testing.T, dir string) map[string]string {
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

func TestProjectsDeleteActiveSeveralRemainingOffTTYClearsProfile(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"Alpha"},{"id":"proj_b","name":"Beta"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "DELETE" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_compat" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if len(reqs) != 3 {
		t.Fatalf("several-remaining off-TTY must fetch the list then stop, got %d: %+v", len(reqs), reqs)
	}
	profile := readProfile(t, dir)
	for _, key := range []string{"active_project", "api_key", "api_secret"} {
		if profile[key] != "" {
			t.Fatalf("deleting the active project must clear %s on disk, got %q", key, profile[key])
		}
	}
	if !bytes.Contains(stdout.Bytes(), []byte("(was your active project)")) {
		t.Fatalf("summary must mark the active project: %s", stdout.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("now_active")) {
		t.Fatalf("off-TTY must not auto-select a survivor: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "urlbox projects select") {
		t.Fatalf("stderr must point at projects select: %q", stderr.String())
	}
}

func TestProjectsDeleteActiveOneRemainingAutoSelects(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_solo","name":"Solo"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_solo","apiSecret":"sk_solo","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[3].Method != "GET" || reqs[3].Path != "/v2/organisation/org_compat/projects/proj_solo/api-credentials" {
		t.Fatalf("survivor credential must be fetched: %+v", reqs[3])
	}
	profile := readProfile(t, dir)
	if profile["active_project"] != "proj_solo" || profile["api_key"] != "pk_solo" || profile["api_secret"] != "sk_solo" {
		t.Fatalf("the lone survivor must become active with its credential: %#v", profile)
	}
	if !strings.Contains(stderr.String(), "Now active: Solo") {
		t.Fatalf("stderr must report the new active project: %q", stderr.String())
	}
	for _, want := range []string{`"now_active"`, `"proj_solo"`, `"Solo"`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("data must carry now_active %s: %s", want, stdout.String())
		}
	}
	if !bytes.Contains(stdout.Bytes(), []byte("(was your active project)")) {
		t.Fatalf("summary must still mark the deleted project: %s", stdout.String())
	}
}

func TestProjectsDeleteActiveSeveralRemainingPickerSelects(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_a","name":"Alpha"},{"id":"proj_b","name":"Beta"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[{"apiKey":"pk_beta","apiSecret":"sk_beta","revoked":false}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	picked := false
	SetDeleteProjectPickForTest(func(_ string, options []string, active int) (int, error) {
		picked = true
		if len(options) != 2 {
			t.Fatalf("picker options = %v, want 2", options)
		}
		if active != 0 {
			t.Fatalf("picker must seed at index 0, got %d", active)
		}
		return 1, nil
	})
	t.Cleanup(ResetDeleteProjectPickForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if !picked {
		t.Fatal("interactive survivor picker was not invoked")
	}
	profile := readProfile(t, dir)
	if profile["active_project"] != "proj_b" || profile["api_key"] != "pk_beta" || profile["api_secret"] != "sk_beta" {
		t.Fatalf("picked survivor must become active with its credential: %#v", profile)
	}
	if !strings.Contains(stderr.String(), "Now active: Beta") {
		t.Fatalf("stderr must report the picked project: %q", stderr.String())
	}
}

func TestProjectsDeleteActiveOneRemainingOffTTYIssuesWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_solo","name":"Solo"}]}`),
		apitest.SuccessJSON(`{"apiCredentials":[]}`),
		apitest.SuccessJSON(`{"apiKey":"pk_solo","apiSecret":"sk_solo"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetDeleteProjectPickForTest(func(_ string, _ []string, _ int) (int, error) {
		t.Fatalf("off-TTY survivor path must not prompt to issue a credential")
		return 0, nil
	})
	t.Cleanup(ResetDeleteProjectPickForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[3].Method != "GET" || reqs[3].Path != "/v2/organisation/org_compat/projects/proj_solo/api-credentials" {
		t.Fatalf("survivor credential must be fetched: %+v", reqs[3])
	}
	if reqs[4].Method != "POST" || reqs[4].Path != "/v2/organisation/org_compat/projects/proj_solo/api-credentials" {
		t.Fatalf("off-TTY survivor lacking a credential must issue one silently: %+v", reqs[4])
	}
	profile := readProfile(t, dir)
	if profile["active_project"] != "proj_solo" || profile["api_key"] != "pk_solo" || profile["api_secret"] != "sk_solo" {
		t.Fatalf("the lone survivor must become active with its issued credential: %#v", profile)
	}
}

func TestProjectsDeleteActiveListErrorFallsBackToPointer(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
		apitest.ServerError(500),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("a failed list re-resolve must not fail the delete: exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	profile := readProfile(t, dir)
	for _, key := range []string{"active_project", "api_key", "api_secret"} {
		if profile[key] != "" {
			t.Fatalf("list error must clear %s on disk, got %q", key, profile[key])
		}
	}
	if bytes.Contains(stdout.Bytes(), []byte("now_active")) {
		t.Fatalf("list error must not auto-select a survivor: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "urlbox projects select") {
		t.Fatalf("stderr must point at projects select: %q", stderr.String())
	}
}

func TestProjectsDeleteNonActiveLeavesProfileUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	before, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_other","name":"Doomed"}]}`),
		apitest.SuccessJSON(`{}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "delete", "doomed", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	after, err := os.ReadFile(filepath.Join(dir, "urlbox", "config.json"))
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("deleting a non-active project must leave the profile bytes unchanged\nbefore: %s\nafter:  %s", before, after)
	}
	if bytes.Contains(stdout.Bytes(), []byte("was your active project")) {
		t.Fatalf("non-active delete must not claim the active project: %s", stdout.String())
	}
}

func TestProjectsDefaultsSetAndRemove(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","defaultOptions":{"format":"png"}}}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "defaults", "set", "main", "--json", `{"format":"png"}`, "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("set exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1/render-defaults" {
		t.Fatalf("set request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"format":"png"`)) {
		t.Fatalf("set body: %s", reqs[1].Body)
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute([]string{"projects", "defaults", "remove", "main", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("remove exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs = srv.Requests()
	if !bytes.Contains(reqs[3].Body, []byte(`"options":null`)) {
		t.Fatalf("remove body: %s", reqs[3].Body)
	}
}

func TestProjectsDefaultsShowReadsFlatShape(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"id":"proj_1","name":"Main","defaultOptions":{"width":1280,"format":"png"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "defaults", "show", "main", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{`"width": 1280`, `"format": "png"`, `2 default options`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("defaults show must read flat defaultOptions, missing %s: %s", want, stdout.String())
		}
	}
}

func TestProjectsDefaultsSetMergeReadsFlatShape(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"id":"proj_1","name":"Main","defaultOptions":{"a":1}}`),
		apitest.SuccessJSON(`{"id":"proj_1"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "defaults", "set", "main", "--merge", "--json", `{"b":2}`, "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	patch := reqs[len(reqs)-1]
	if patch.Method != "PATCH" || patch.Path != "/v2/organisation/org_compat/projects/proj_1/render-defaults" {
		t.Fatalf("merge PATCH request: %+v", patch)
	}
	for _, want := range []string{`"a":1`, `"b":2`} {
		if !bytes.Contains(patch.Body, []byte(want)) {
			t.Fatalf("merge must PATCH existing + new keys, missing %s: %s", want, patch.Body)
		}
	}
}

func TestProjectsDefaultsRemoveRequiresYesOffTTY(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "defaults", "remove", "main", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("remove without --yes off-TTY must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("--yes")) {
		t.Fatalf("error must name --yes: %s", stdout.String())
	}
	for _, r := range srv.Requests() {
		if r.Method == "PATCH" {
			t.Fatalf("no PATCH must be made without confirmation: %+v", r)
		}
	}
}

func TestProjectsDisableRequiresYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("disable without --yes must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("--yes")) {
		t.Fatalf("error must name --yes: %s", stdout.String())
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("no API call must be made without --yes: %+v", srv.Requests())
	}
}

func TestProjectsDisableWithYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","enabled":false}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"enabled":false`)) {
		t.Fatalf("body: %s", reqs[1].Body)
	}
}

func TestProjectsDisableAcceptedConfirmDisables(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","enabled":false}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetConfirmPromptForTest(func(string) (bool, error) { return true, nil })
	t.Cleanup(ResetConfirmPromptForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("accepted confirm must resolve then PATCH, got %d: %+v", len(reqs), reqs)
	}
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"enabled":false`)) {
		t.Fatalf("body: %s", reqs[1].Body)
	}
}

func TestProjectsDisableDeclinedConfirmMakesNoPatch(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetConfirmPromptForTest(func(string) (bool, error) { return false, nil })
	t.Cleanup(ResetConfirmPromptForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("declining the disable must exit 0, got %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, r := range srv.Requests() {
		if r.Method == "PATCH" {
			t.Fatalf("declined disable must make no PATCH: %+v", r)
		}
	}
	if !strings.Contains(stderr.String(), "Left Main enabled.") {
		t.Fatalf("declined disable must report the project was left enabled: %q", stderr.String())
	}
}

func TestProjectsDisableAlreadyDisabledByNameSkipsConfirmAndPatch(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main","enabled":false}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetConfirmPromptForTest(func(string) (bool, error) {
		t.Fatalf("already-disabled project must not prompt for confirmation")
		return false, nil
	})
	t.Cleanup(ResetConfirmPromptForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("already-disabled must exit 0, got %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("already-disabled must fetch the list only, got %d: %+v", len(reqs), reqs)
	}
	if reqs[0].Method == "PATCH" {
		t.Fatalf("already-disabled must make no PATCH: %+v", reqs[0])
	}
	if !bytes.Contains(stdout.Bytes(), []byte("is already disabled")) {
		t.Fatalf("summary must state the project is already disabled: %s", stdout.String())
	}
}

func TestProjectsDisableEnabledByNameConfirmsAndPatches(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main","enabled":true}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","enabled":false}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	SetConfirmPromptForTest(func(string) (bool, error) { return true, nil })
	t.Cleanup(ResetConfirmPromptForTest)

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("enabled project must resolve then PATCH, got %d: %+v", len(reqs), reqs)
	}
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"enabled":false`)) {
		t.Fatalf("body: %s", reqs[1].Body)
	}
}

func TestProjectsDisableAlreadyDisabledByPrefixedIDPatches(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main","enabled":false}]}`),
		apitest.SuccessJSON(`{"project":{"id":"proj_1","enabled":false}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "proj_1", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("prefixed-id disable must not skip the PATCH, got %d: %+v", len(reqs), reqs)
	}
	if reqs[1].Method != "PATCH" || reqs[1].Path != "/v2/organisation/org_compat/projects/proj_1" {
		t.Fatalf("request: %+v", reqs[1])
	}
	if !bytes.Contains(reqs[1].Body, []byte(`"enabled":false`)) {
		t.Fatalf("body: %s", reqs[1].Body)
	}
}

func TestProjectsDisableAlreadyDisabledByNameWithYesSkipsPatch(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main","enabled":false}]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "disable", "main", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("already-disabled with --yes must exit 0, got %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("already-disabled with --yes must fetch the list only, got %d: %+v", len(reqs), reqs)
	}
	if reqs[0].Method == "PATCH" {
		t.Fatalf("already-disabled with --yes must make no PATCH: %+v", reqs[0])
	}
	if !bytes.Contains(stdout.Bytes(), []byte("is already disabled")) {
		t.Fatalf("summary must state the project is already disabled: %s", stdout.String())
	}
}

func TestProjectsCrudNeedsActiveOrg(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfigNoOrg(t, dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_HOST", "http://127.0.0.1:1")
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "rename", "proj_x", "New", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("no active org must be usage exit 1, got %d\n%s", code, stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("orgs select")) {
		t.Fatalf("hint must name orgs select: %s", stdout.String())
	}
}
