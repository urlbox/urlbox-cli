package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const llmListJSON = `{"llmCredentials":[
 {"id":"llm_1","name":"openai-prod","provider":"openai","model":"gpt-5","baseUrl":null,"apiKey":"sk-verysecretkey123","awsAccessKeyId":null,"awsSecretAccessKey":null,"awsSessionToken":null,"gcpServiceAccountJson":null,"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"},
 {"id":"llm_2","name":"bedrock","provider":"amazon-bedrock","model":null,"baseUrl":null,"apiKey":null,"awsAccessKeyId":"AKIAFAKEFAKEFAKE","awsSecretAccessKey":"sk_fake_aws_secret_value","awsSessionToken":"tok_fake_session","gcpServiceAccountJson":null,"assignedProjectIds":[],"createdAt":"2026-08-02T00:00:00.000Z"}]}`

const llmOneJSON = `{"id":"llm_2","name":"bedrock","provider":"amazon-bedrock","model":"claude-3","baseUrl":null,"apiKey":null,"awsAccessKeyId":"AKIAFAKEFAKEFAKE","awsSecretAccessKey":"sk_fake_aws_secret_value","awsSessionToken":"tok_fake_session","gcpServiceAccountJson":"{\"type\":\"service_account\"}","assignedProjectIds":[],"createdAt":"2026-08-02T00:00:00.000Z"}`

func TestLlmListRendersTable(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(llmListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "list", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "GET" || reqs[0].Path != "/v2/organisation/org_compat/llm-credentials" {
		t.Fatalf("request: %+v", reqs[0])
	}
	out := stdout.String()
	for _, want := range []string{"ID", "NAME", "PROVIDER", "MODEL", "ASSIGNED", "llm_1", "openai-prod", "openai", "gpt-5", "amazon-bedrock"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("list output missing %q: %s", want, out)
		}
	}
	if bytes.Contains(stdout.Bytes(), []byte("sk-verysecretkey123")) {
		t.Fatalf("list must never print the api key: %s", out)
	}
}

func TestLlmShowMasksSecretsRevealUnhides(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(llmListJSON),
		apitest.SuccessJSON(llmOneJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "show", "llm_2", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "GET" || reqs[1].Path != "/v2/organisation/org_compat/llm-credentials/llm_2" {
		t.Fatalf("show request: %+v", reqs[1])
	}
	out := stdout.String()
	for _, want := range []string{"NAME", "ID", "PROVIDER", "amazon-bedrock"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("show output missing %q: %s", want, out)
		}
	}
	for _, secret := range []string{"AKIAFAKEFAKEFAKE", "sk_fake_aws_secret_value", "tok_fake_session", "service_account"} {
		if bytes.Contains(stdout.Bytes(), []byte(secret)) {
			t.Fatalf("show must mask %q without --reveal: %s", secret, out)
		}
	}

	srv2 := apitest.New(
		apitest.SuccessJSON(llmListJSON),
		apitest.SuccessJSON(llmOneJSON),
	)
	t.Cleanup(srv2.Close)
	t.Setenv("URLBOX_API_HOST", srv2.URL())
	var revealOut, revealErr bytes.Buffer
	code = Execute([]string{"llm", "show", "llm_2", "--reveal", "--output-format", "text"}, &revealOut, &revealErr)
	if code != 0 {
		t.Fatalf("reveal exit %d\n%s\n%s", code, revealOut.String(), revealErr.String())
	}
	for _, want := range []string{"AKIAFAKEFAKEFAKE", "sk_fake_aws_secret_value", "tok_fake_session"} {
		if !bytes.Contains(revealOut.Bytes(), []byte(want)) {
			t.Fatalf("--reveal must show full %q: %s", want, revealOut.String())
		}
	}
}

func TestLlmCreateSendsTypedFlagsAndJSONMerge(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"llm_new","name":"x","provider":"openai","assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"llm", "create",
		"--name", "x", "--provider", "openai", "--api-key", "sk-1",
		"--model", "gpt-5", "--base-url", "https://api.example.com",
		"--json", `{"temperature":0.2}`,
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/llm-credentials" {
		t.Fatalf("create request: %+v", reqs[0])
	}
	for _, want := range []string{
		`"name":"x"`, `"provider":"openai"`, `"apiKey":"sk-1"`,
		`"model":"gpt-5"`, `"baseUrl":"https://api.example.com"`, `"temperature":0.2`,
	} {
		if !bytes.Contains(reqs[0].Body, []byte(want)) {
			t.Fatalf("create body missing %s: %s", want, reqs[0].Body)
		}
	}
}

func TestLlmCreatePositionalName(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"llm_new","name":"openai","provider":"openai","assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"llm", "create", "openai",
		"--provider", "openai", "--api-key", "sk-1",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/llm-credentials" {
		t.Fatalf("create request: %+v", reqs[0])
	}
	if !bytes.Contains(reqs[0].Body, []byte(`"name":"openai"`)) {
		t.Fatalf("create body must carry the positional name: %s", reqs[0].Body)
	}
}

func TestLlmCreatePositionalConflictsWithFlag(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"llm", "create", "a", "--name", "b",
		"--provider", "openai", "--api-key", "sk-1",
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

func TestLlmUpdatePartialPatch(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"llm_1","name":"openai-prod","model":"gpt-5-mini"}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "update", "llm_1", "--model", "gpt-5-mini", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("prefixed-id update must make no list call, got %d requests: %+v", len(reqs), reqs)
	}
	if reqs[0].Method != "PATCH" || reqs[0].Path != "/v2/organisation/org_compat/llm-credentials/llm_1" {
		t.Fatalf("update request: %+v", reqs[0])
	}
	if string(reqs[0].Body) != `{"model":"gpt-5-mini"}` {
		t.Fatalf("update body must be exactly the changed field: %s", reqs[0].Body)
	}
}

func TestLlmUpdateProviderIsImmutable(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New()
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "update", "llm_1", "--provider", "anthropic", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("changing --provider on update must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("provider is immutable after create — create a new credential instead")) {
		t.Fatalf("error must state provider is immutable: %s", stdout.String())
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("immutable-provider rejection must be client-side, got requests: %+v", srv.Requests())
	}
}

func TestLlmCreateBedrockRequiresAwsFields(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New()
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "create", "--name", "b", "--provider", "amazon-bedrock", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("bedrock create without AWS fields must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("amazon-bedrock requires awsRegion, awsAccessKeyId, awsSecretAccessKey — pass them via --json")) {
		t.Fatalf("error must name the missing bedrock fields: %s", stdout.String())
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("validation must be client-side, got requests: %+v", srv.Requests())
	}
}

func TestLlmDeleteRequiresYesOffTTY(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(llmListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "delete", "llm_1", "--output-format", "json"}, &stdout, &stderr)
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

func TestLlmDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(llmListJSON),
		apitest.SuccessJSON(`{"id":"llm_1","deleted":true}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "delete", "llm_1", "--yes", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Method != "DELETE" || last.Path != "/v2/organisation/org_compat/llm-credentials/llm_1" {
		t.Fatalf("delete request: %+v", last)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("openai-prod")) {
		t.Fatalf("delete summary must name the credential: %s", stdout.String())
	}
}

func TestLlmCreateAssignTo(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"id":"llm_new","name":"x","provider":"openai","assignedProjectIds":[]}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"id":"proj_1","name":"Main","llmCredentialId":"llm_new"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"llm", "create",
		"--name", "x", "--provider", "openai", "--api-key", "sk-1",
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
	if put.Path != "/v2/organisation/org_compat/projects/proj_1/llm-credential" {
		t.Fatalf("assign PUT path: %s", put.Path)
	}
	if !bytes.Contains(put.Body, []byte(`"llmCredentialId":"llm_new"`)) {
		t.Fatalf("assign body missing llmCredentialId: %s", put.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"assigned"`)) {
		t.Fatalf("envelope data must carry the assigned project: %s", stdout.String())
	}
}

func TestLlmNotFoundNameHint(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(llmListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "show", "nope", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("show of an unknown name must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("urlbox llm list")) {
		t.Fatalf("not-found hint must name `urlbox llm list`: %s", stdout.String())
	}
}

func TestLlmTestOkAndError(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"ok":true}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "test", "llm_1", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("ok test must exit 0\n%s\n%s", stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Method != "POST" || last.Path != "/v2/organisation/org_compat/llm-credentials/llm_1/test" {
		t.Fatalf("test request: %+v", last)
	}
	if string(last.Body) != `{}` {
		t.Fatalf("test body must be an empty object: %s", last.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("Connection OK")) {
		t.Fatalf("ok test must render Connection OK: %s", stdout.String())
	}

	srv2 := apitest.New(apitest.SuccessJSON(`{"ok":false,"error":"bad key"}`))
	t.Cleanup(srv2.Close)
	t.Setenv("URLBOX_API_HOST", srv2.URL())
	var failOut, failErr bytes.Buffer
	code = Execute([]string{"llm", "test", "llm_1", "--output-format", "text"}, &failOut, &failErr)
	if code != 1 {
		t.Fatalf("failed test must exit 1, got %d\n%s\n%s", code, failOut.String(), failErr.String())
	}
	if !bytes.Contains(failOut.Bytes(), []byte("bad key")) {
		t.Fatalf("failed test summary must carry the reason: %s", failOut.String())
	}
}

func TestLlmTestErrorJSONEnvelopeNotOk(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"ok":false,"error":"bad key"}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "test", "llm_1", "--output-format", "json"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("failed test must exit 1, got %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"ok": false`)) {
		t.Fatalf("json envelope for a failed test must be ok:false: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("bad key")) {
		t.Fatalf("json envelope must carry the error: %s", stdout.String())
	}
}

func TestLlmModels(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"models":["gpt-a","gpt-b"]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "models", "llm_1", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Method != "POST" || last.Path != "/v2/organisation/org_compat/llm-credentials/llm_1/models" {
		t.Fatalf("models request: %+v", last)
	}
	if string(last.Body) != `{}` {
		t.Fatalf("models body must be an empty object: %s", last.Body)
	}
	out := stdout.String()
	for _, want := range []string{"MODEL", "gpt-a", "gpt-b"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("models output missing %q: %s", want, out)
		}
	}
}
