package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

const projectShowDetailJSON = `{
  "createdAt": "2026-08-14T10:18:34.436Z",
  "defaultOptions": null,
  "enabled": true,
  "engineVersion": "latest",
  "id": "proj_o5xpfa7z94",
  "llmCredentialId": null,
  "mongoProjectId": null,
  "name": "plan1-manual-check",
  "proxyId": null,
  "region": null,
  "renderQueue": null,
  "storageCredentialId": null,
  "tokenless": true,
  "webhookKey": "ubx_whk_FAKE0000EXAMPLE0000"
}`

func TestProjectsShow_TextMode_KVRendersFromFlatShape(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_o5xpfa7z94","name":"plan1-manual-check"}]}`),
		apitest.SuccessJSON(projectShowDetailJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	out, stderrOut, code := runTextMode(t, "projects", "show", "plan1-manual-check")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	for _, want := range []string{"plan1-manual-check", "proj_o5xpfa7z94", "NAME", "ID", "WEBHOOK KEY"} {
		if !strings.Contains(out, want) {
			t.Errorf("projects show text KV missing %q, got:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "ubx_…00") {
		t.Errorf("webhook key should be masked, got:\n%s", out)
	}
	if strings.Contains(out, "ubx_whk_FAKE0000EXAMPLE0000") {
		t.Errorf("full webhook key must not appear without --reveal, got:\n%s", out)
	}
}

func TestProjectsShow_TextMode_KVSurfacesNameNoSummaryLine(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_o5xpfa7z94","name":"plan1-manual-check"}]}`),
		apitest.SuccessJSON(projectShowDetailJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	out, stderrOut, code := runTextMode(t, "projects", "show", "plan1-manual-check")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	if !strings.Contains(out, "plan1-manual-check") {
		t.Errorf("KV view should surface the project name, got:\n%s", out)
	}
	if strings.Contains(out, "Project plan1-manual-check") {
		t.Errorf("ok+view should omit the 'Project <name>' summary line, got:\n%s", out)
	}
}

func TestProjectsShow_Reveal_ShowsFullWebhookKey(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_o5xpfa7z94","name":"plan1-manual-check"}]}`),
		apitest.SuccessJSON(projectShowDetailJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	out, stderrOut, code := runTextMode(t, "projects", "show", "plan1-manual-check", "--reveal")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	if !strings.Contains(out, "ubx_whk_FAKE0000EXAMPLE0000") {
		t.Errorf("--reveal should print the full webhook key, got:\n%s", out)
	}
}

func TestProjectsShow_JSON_ByteIdenticalToServerResponse(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"projects":[{"id":"proj_o5xpfa7z94","name":"plan1-manual-check"}]}`),
		apitest.SuccessJSON(projectShowDetailJSON),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	var stdout, stderr bytes.Buffer
	code := Execute([]string{"projects", "show", "plan1-manual-check", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal envelope: %v\n%s", err, stdout.String())
	}
	var got, want map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if err := json.Unmarshal([]byte(projectShowDetailJSON), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if !bytes.Equal(gb, wb) {
		t.Errorf("JSON data must be the raw server response.\n got: %s\nwant: %s", gb, wb)
	}
	if bytes.Contains(env.Data, []byte("ubx_…")) {
		t.Errorf("JSON output must carry the verbatim webhook key, not the masked form:\n%s", env.Data)
	}
}
