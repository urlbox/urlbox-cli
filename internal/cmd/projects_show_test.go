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

func TestProjectsShow_JSON_PassesServerResponseThroughWithSecretsMasked(t *testing.T) {
	show := func(t *testing.T, args ...string) json.RawMessage {
		t.Helper()
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
		if code := Execute(args, &stdout, &stderr); code != 0 {
			t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
		}
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
			t.Fatalf("unmarshal envelope: %v\n%s", err, stdout.String())
		}
		return env.Data
	}

	var want map[string]any
	if err := json.Unmarshal([]byte(projectShowDetailJSON), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	// Default: every field is the raw server response except the credential
	// material, which is masked the same way the text view masks it.
	var got map[string]any
	masked := show(t, "projects", "show", "plan1-manual-check", "--output-format", "json")
	if err := json.Unmarshal(masked, &got); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if got["webhookKey"] == want["webhookKey"] {
		t.Errorf("webhookKey must be masked by default: %s", masked)
	}
	delete(got, "webhookKey")
	delete(want, "webhookKey")
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if !bytes.Equal(gb, wb) {
		t.Errorf("non-secret fields must pass through untouched.\n got: %s\nwant: %s", gb, wb)
	}

	// --reveal returns the response verbatim, webhook key included.
	revealed := show(t, "projects", "show", "plan1-manual-check", "--reveal", "--output-format", "json")
	var full, fixture map[string]any
	if err := json.Unmarshal(revealed, &full); err != nil {
		t.Fatalf("unmarshal revealed: %v", err)
	}
	if err := json.Unmarshal([]byte(projectShowDetailJSON), &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	fb, _ := json.Marshal(full)
	xb, _ := json.Marshal(fixture)
	if !bytes.Equal(fb, xb) {
		t.Errorf("--reveal must return the raw server response.\n got: %s\nwant: %s", fb, xb)
	}
}
