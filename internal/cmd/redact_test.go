package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

// The JSON envelope is what agents and pipelines read: `urlbox storage list`
// with stdout on a pipe resolves to JSON without any flag. These tests pin the
// rule that credential material is masked there by default, exactly as it is in
// the text views, and that --reveal is the single switch that unhides both.

func TestStorageListJSONMasksSecretsByDefault(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(storageListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, secret := range []string{"AKIAFAKEFAKEFAKE", "sk_fake_secret_value", "sv=fake"} {
		if bytes.Contains(stdout.Bytes(), []byte(secret)) {
			t.Fatalf("JSON list leaked %q: %s", secret, stdout.String())
		}
	}
	// Non-secret fields survive untouched.
	for _, want := range []string{`"prod-bucket"`, `"store_1"`, `"us-east-1"`} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("JSON list dropped %s: %s", want, stdout.String())
		}
	}
}

func TestStorageListJSONRevealUnhides(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(storageListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "list", "--reveal", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, secret := range []string{"AKIAFAKEFAKEFAKE", "sk_fake_secret_value"} {
		if !bytes.Contains(stdout.Bytes(), []byte(secret)) {
			t.Fatalf("--reveal must show %q: %s", secret, stdout.String())
		}
	}
}

func TestStorageShowJSONMasksSecretsRevealUnhides(t *testing.T) {
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		dir := t.TempDir()
		writeCompatConfig(t, dir, true)
		t.Setenv("XDG_CONFIG_HOME", dir)
		srv := apitest.New(
			apitest.SuccessJSON(storageListJSON),
			apitest.SuccessJSON(storageOneJSON),
		)
		t.Cleanup(srv.Close)
		t.Setenv("URLBOX_API_HOST", srv.URL())
		var stdout, stderr bytes.Buffer
		if code := Execute(args, &stdout, &stderr); code != 0 {
			t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
		}
		return stdout.String()
	}
	masked := run(t, "storage", "show", "store_1", "--output-format", "json")
	if bytes.Contains([]byte(masked), []byte("sk_fake_secret_value")) {
		t.Fatalf("JSON show leaked the secret: %s", masked)
	}
	revealed := run(t, "storage", "show", "store_1", "--reveal", "--output-format", "json")
	if !bytes.Contains([]byte(revealed), []byte("sk_fake_secret_value")) {
		t.Fatalf("--reveal must show the secret: %s", revealed)
	}
}

func TestLLMJSONMasksEveryCredentialField(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	body := `{"llmCredentials":[{"id":"llm_1","name":"main","provider":"openai","model":"gpt-4o",` +
		`"apiKey":"sk-leakme000000","awsAccessKeyId":"AKIALEAK0000","awsSecretAccessKey":"awssecretleak",` +
		`"awsSessionToken":"sessleak","gcpServiceAccountJson":"{\"private_key\":\"gcpleak\"}","assignedProjectIds":[]}]}`
	srv := apitest.New(apitest.SuccessJSON(body))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"llm", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, secret := range []string{"sk-leakme000000", "AKIALEAK0000", "awssecretleak", "sessleak", "gcpleak"} {
		if bytes.Contains(stdout.Bytes(), []byte(secret)) {
			t.Fatalf("JSON llm list leaked %q: %s", secret, stdout.String())
		}
	}
}

func TestProxiesJSONMasksPasswordKeepsHost(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	body := `{"proxies":[{"id":"pool_1","name":"eu","assignedProjectIds":[],` +
		`"proxies":[{"url":"http://user:hunter2@proxy.example.com:8080"}]}]}`
	srv := apitest.New(apitest.SuccessJSON(body))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"proxies", "list", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("hunter2")) {
		t.Fatalf("JSON proxies list leaked the password: %s", stdout.String())
	}
	// The host stays legible — masking the password must not blind the operator.
	if !bytes.Contains(stdout.Bytes(), []byte("proxy.example.com:8080")) {
		t.Fatalf("JSON proxies list dropped the host: %s", stdout.String())
	}
}

func TestProjectsShowJSONMasksWebhookKey(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"prod"}]}`),
		apitest.SuccessJSON(`{"id":"proj_1","name":"prod","enabled":true,"webhookKey":"whk_leakme00000"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "show", "proj_1", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("whk_leakme00000")) {
		t.Fatalf("JSON projects show leaked the webhook key: %s", stdout.String())
	}
}
