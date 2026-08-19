package cmd

import (
	"bytes"
	"testing"
)

func TestConfigGetSessionTokenMasked(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "get", "session_token", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if bytes.Contains(stdout.Bytes(), []byte("sess_tok_compat_123456")) {
		t.Fatalf("session token leaked unmasked: %s", stdout.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("sess")) {
		t.Fatalf("masked value missing: %s", stdout.String())
	}
}

func TestConfigGetSessionTokenReveal(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "get", "session_token", "--reveal", "--output-format", "json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr.String())
	}
	if !bytes.Contains(stdout.Bytes(), []byte("sess_tok_compat_123456")) {
		t.Fatalf("--reveal must show the token: %s", stdout.String())
	}
}

func TestConfigGetActiveOrgAndProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	for key, want := range map[string]string{"active_org": "org_compat", "active_project": "proj_compat"} {
		var stdout, stderr bytes.Buffer
		code := Execute([]string{"config", "get", key, "--output-format", "json"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("%s exit %d", key, code)
		}
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("%s: %s", key, stdout.String())
		}
	}
}

func TestConfigSetActiveProject(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"config", "set", "active_project", "proj_other"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, stderr.String())
	}
	if p := readProfileMap(t, dir); p["active_project"] != "proj_other" {
		t.Fatalf("profile: %#v", p)
	}
}
