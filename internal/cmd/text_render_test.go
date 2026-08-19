package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/api/apitest"
)

func runTextMode(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	t.Setenv("NO_COLOR", "1")
	var out, errBuf bytes.Buffer
	code = Execute(append(args, "--output-format", "text"), &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestOrgsList_TextMode_TableWithMarker(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`[{"id":"1","name":"One","publicId":"org_one"},{"id":"2","name":"Two","publicId":"org_two"}]`),
		apitest.SuccessJSON(`{"user":{"email":"a@urlbox.com"},"session":{"activeOrganizationId":"2","activeOrganizationPublicId":"org_two"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	out, stderrOut, code := runTextMode(t, "orgs", "list")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	for _, want := range []string{"One", "Two", "org_one", "org_two", "●"} {
		if !strings.Contains(out, want) {
			t.Errorf("orgs list text missing %q, got:\n%s", want, out)
		}
	}
	activeLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Two") {
			activeLine = line
		}
	}
	if !strings.Contains(activeLine, "●") {
		t.Errorf("active marker should be on the Two row, got %q", activeLine)
	}
}

func TestWhoami_TextMode_KVContainsEmail(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"user":{"email":"me@urlbox.com"},"session":{"activeOrganizationId":"1","activeOrganizationPublicId":"org_one"}}`),
		apitest.SuccessJSON(`[{"id":"1","name":"Acme","publicId":"org_one"}]`),
		apitest.SuccessJSON(`{"projects":[{"id":"proj_compat","name":"Prod"}]}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	out, stderrOut, code := runTextMode(t, "whoami")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	if !strings.Contains(out, "me@urlbox.com") {
		t.Errorf("whoami text KV should contain email, got:\n%s", out)
	}
	if !strings.Contains(out, "SIGNED IN") {
		t.Errorf("whoami text KV should carry the uppercased 'SIGNED IN' label, got:\n%s", out)
	}
}

func TestUsage_TextMode_KVContainsFields(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	srv := apitest.New(
		apitest.SuccessJSON(`{"rendersUsed":42,"renderQuota":1000,"period":{"start":"2026-08-01","end":"2026-08-31"}}`),
	)
	t.Cleanup(srv.Close)
	t.Setenv("URLBOX_API_HOST", srv.URL())

	out, stderrOut, code := runTextMode(t, "usage")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	for _, want := range []string{"42", "1000", "2026-08-01"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage text KV missing %q, got:\n%s", want, out)
		}
	}
}

func TestLink_TextMode_ContainsFullSignedURL(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)

	out, stderrOut, code := runTextMode(t, "link", "https://example.com")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, stderrOut)
	}
	if !strings.Contains(out, "/v1/pk_test_key/") {
		t.Errorf("link text should contain the full signed URL path, got:\n%s", out)
	}
	if !strings.Contains(out, "url=https") {
		t.Errorf("link text should contain the encoded url query, got:\n%s", out)
	}
	if !strings.Contains(out, "URL") {
		t.Errorf("link text KV should carry a URL label, got:\n%s", out)
	}
}

func TestDoctor_TextMode_FailureShowsGlyphAndHint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("URLBOX_API_HOST", "http://127.0.0.1:1")

	out, _, code := runTextMode(t, "doctor")
	if code == 0 {
		t.Fatalf("doctor with no secret + unreachable host should fail, got exit 0:\n%s", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("doctor failure summary should use ✗, got:\n%s", out)
	}
	if strings.Contains(strings.SplitN(out, "\n", 2)[0], "✓") {
		t.Errorf("doctor failure top line must not print ✓, got:\n%s", out)
	}
	if !strings.Contains(out, "render_credential") {
		t.Errorf("doctor text should list per-check names, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "hint") {
		t.Errorf("doctor text should surface a hint line for the failing check, got:\n%s", out)
	}
}
