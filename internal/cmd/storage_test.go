package cmd

import (
	"bytes"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const storageListJSON = `{"storageCredentials":[
 {"id":"store_1","type":"s3","name":"prod","provider":"aws_s3","bucket":"prod-bucket","region":"us-east-1","endpoint":null,"privateBucket":false,"objectLock":false,"accountName":null,"containerName":null,"cdnHost":null,"key":"AKIAFAKEFAKEFAKE","secret":"sk_fake_secret_value","sasToken":null,"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"},
 {"id":"store_2","type":"azure","name":"az","provider":null,"bucket":null,"region":null,"endpoint":null,"privateBucket":true,"objectLock":false,"accountName":"acct","containerName":"cont","cdnHost":null,"key":null,"secret":null,"sasToken":"sv=fake","assignedProjectIds":[],"createdAt":"2026-08-02T00:00:00.000Z"}]}`

const storageOneJSON = `{"id":"store_1","type":"s3","name":"prod","provider":"aws_s3","bucket":"prod-bucket","region":"us-east-1","endpoint":null,"privateBucket":false,"objectLock":false,"accountName":null,"containerName":null,"cdnHost":null,"key":"AKIAFAKEFAKEFAKE","secret":"sk_fake_secret_value","sasToken":null,"assignedProjectIds":["proj_1"],"createdAt":"2026-08-01T00:00:00.000Z"}`

func TestStorageListRendersTableMaskedKey(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(storageListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "list", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "GET" || reqs[0].Path != "/v2/organisation/org_compat/storage-credentials" {
		t.Fatalf("request: %+v", reqs[0])
	}
	out := stdout.String()
	for _, want := range []string{"prod-bucket", "store_1", "AWS S3", "AWS S3 (default)", "cont", "Azure"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("list output missing %q: %s", want, out)
		}
	}
	if bytes.Contains(stdout.Bytes(), []byte("AKIAFAKEFAKEFAKE")) {
		t.Fatalf("list must mask the key, raw value present: %s", out)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("…")) {
		t.Fatalf("list must show a masked key with an ellipsis: %s", out)
	}
}

func TestStorageShowMasksSecretsRevealUnhides(t *testing.T) {
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
	code := Execute([]string{"storage", "show", "store_1", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[1].Method != "GET" || reqs[1].Path != "/v2/organisation/org_compat/storage-credentials/store_1" {
		t.Fatalf("show request: %+v", reqs[1])
	}
	out := stdout.String()
	for _, want := range []string{"NAME", "ID", "PROVIDER", "BUCKET", "VISIBILITY", "public"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("show output missing %q: %s", want, out)
		}
	}
	if bytes.Contains(stdout.Bytes(), []byte("AKIAFAKEFAKEFAKE")) {
		t.Fatalf("show must mask the key without --reveal: %s", out)
	}
	if bytes.Contains(stdout.Bytes(), []byte("sk_fake_secret_value")) {
		t.Fatalf("show must mask the secret without --reveal: %s", out)
	}

	srv2 := apitest.New(
		apitest.SuccessJSON(storageListJSON),
		apitest.SuccessJSON(storageOneJSON),
	)
	t.Cleanup(srv2.Close)
	t.Setenv("URLBOX_API_HOST", srv2.URL())
	var revealOut, revealErr bytes.Buffer
	code = Execute([]string{"storage", "show", "store_1", "--reveal", "--output-format", "text"}, &revealOut, &revealErr)
	if code != 0 {
		t.Fatalf("reveal exit %d\n%s\n%s", code, revealOut.String(), revealErr.String())
	}
	for _, want := range []string{"AKIAFAKEFAKEFAKE", "sk_fake_secret_value"} {
		if !bytes.Contains(revealOut.Bytes(), []byte(want)) {
			t.Fatalf("--reveal must show full %q: %s", want, revealOut.String())
		}
	}
}

func TestStorageCreateSendsProviderDrivenBody(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"store_new","type":"s3","name":"x","provider":"cloudflare_r2","bucket":"b","assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"storage", "create",
		"--name", "x", "--provider", "cloudflare_r2", "--bucket", "b", "--region", "auto",
		"--key", "k", "--secret", "s", "--endpoint", "https://x.r2.cloudflarestorage.com",
		"--cdn-host", "cdn.example.com",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/storage-credentials" {
		t.Fatalf("create request: %+v", reqs[0])
	}
	for _, want := range []string{
		`"type":"s3"`, `"provider":"cloudflare_r2"`, `"name":"x"`, `"bucket":"b"`,
		`"region":"auto"`, `"key":"k"`, `"secret":"s"`,
		`"endpoint":"https://x.r2.cloudflarestorage.com"`, `"cdnHost":"cdn.example.com"`,
	} {
		if !bytes.Contains(reqs[0].Body, []byte(want)) {
			t.Fatalf("create body missing %s: %s", want, reqs[0].Body)
		}
	}
}

func TestStorageCreatePositionalName(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"store_new","type":"s3","name":"prod","provider":"aws_s3","bucket":"b","assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"storage", "create", "prod",
		"--provider", "aws_s3", "--bucket", "b", "--region", "us-east-1",
		"--key", "k", "--secret", "s",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if reqs[0].Method != "POST" || reqs[0].Path != "/v2/organisation/org_compat/storage-credentials" {
		t.Fatalf("create request: %+v", reqs[0])
	}
	if !bytes.Contains(reqs[0].Body, []byte(`"name":"prod"`)) {
		t.Fatalf("create body must carry the positional name: %s", reqs[0].Body)
	}
}

func TestStorageCreatePositionalConflictsWithFlag(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"storage", "create", "a", "--name", "b",
		"--provider", "aws_s3", "--bucket", "x", "--region", "us-east-1", "--key", "k", "--secret", "s",
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

func TestStorageUpdatePartialPatch(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"store_1","name":"prod","region":"eu-west-1"}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "update", "store_1", "--region", "eu-west-1", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("prefixed-id update must make no list call, got %d requests: %+v", len(reqs), reqs)
	}
	if reqs[0].Method != "PATCH" || reqs[0].Path != "/v2/organisation/org_compat/storage-credentials/store_1" {
		t.Fatalf("update request: %+v", reqs[0])
	}
	if string(reqs[0].Body) != `{"region":"eu-west-1"}` {
		t.Fatalf("update body must be exactly the changed field: %s", reqs[0].Body)
	}
}

func TestStorageDeleteRequiresYesOffTTY(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(storageListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "delete", "store_1", "--output-format", "json"}, &stdout, &stderr)
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

func TestStorageDeleteWithYes(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(storageListJSON),
		apitest.SuccessJSON(`{"id":"store_1","deleted":true}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "delete", "store_1", "--yes", "--output-format", "text"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	reqs := srv.Requests()
	last := reqs[len(reqs)-1]
	if last.Method != "DELETE" || last.Path != "/v2/organisation/org_compat/storage-credentials/store_1" {
		t.Fatalf("delete request: %+v", last)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("prod")) {
		t.Fatalf("delete summary must name the credential: %s", stdout.String())
	}
}

func TestStorageCreateAssignTo(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"id":"store_new","type":"s3","name":"x","provider":"aws_s3","bucket":"b","assignedProjectIds":[]}`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_1","name":"Main"}]}`),
		apitest.SuccessJSON(`{"id":"proj_1","name":"Main","storageCredentialId":"store_new"}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"storage", "create",
		"--name", "x", "--provider", "aws_s3", "--bucket", "b", "--region", "us-east-1",
		"--key", "k", "--secret", "s",
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
	if put.Path != "/v2/organisation/org_compat/projects/proj_1/storage-credential" {
		t.Fatalf("assign PUT path: %s", put.Path)
	}
	if !bytes.Contains(put.Body, []byte(`"storageCredentialId":"store_new"`)) {
		t.Fatalf("assign body missing storageCredentialId: %s", put.Body)
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"assigned"`)) {
		t.Fatalf("envelope data must carry the assigned project: %s", stdout.String())
	}
}

func TestStorageCreateNoAssignHasNilAssigned(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(`{"id":"store_new","type":"s3","name":"x","provider":"aws_s3","bucket":"b","assignedProjectIds":[]}`))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{
		"storage", "create",
		"--name", "x", "--provider", "aws_s3", "--bucket", "b", "--region", "us-east-1",
		"--key", "k", "--secret", "s",
		"--output-format", "json",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	for _, r := range srv.Requests() {
		if r.Method == "PUT" {
			t.Fatalf("no assign must happen without --assign-to: %+v", r)
		}
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"assigned": null`)) {
		t.Fatalf("envelope data must carry assigned:null when not assigned: %s", stdout.String())
	}
}

func TestStorageNotFoundNameHint(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(apitest.SuccessJSON(storageListJSON))
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"storage", "show", "nope", "--output-format", "json"}, &stdout, &stderr)
	if code == 0 {
		t.Fatalf("show of an unknown name must fail\n%s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("urlbox storage list")) {
		t.Fatalf("not-found hint must name `urlbox storage list`: %s", stdout.String())
	}
}
