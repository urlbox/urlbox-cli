package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const compatSecret = "sk_test_abcdefgh12345678"

// unreachableHost is a closed port on loopback: connection-refused is
// immediate and identical regardless of which config state is loaded, so
// commands that must reach the network fail fast and deterministically.
const unreachableHost = "http://127.0.0.1:1"

func writeCompatConfig(t *testing.T, dir string, withSession bool) {
	t.Helper()
	profile := map[string]string{
		"api_key":    "pk_test_key",
		"api_secret": compatSecret,
	}
	if withSession {
		profile["session_token"] = "sess_tok_compat_123456"
		profile["active_org"] = "org_compat"
		profile["active_project"] = "proj_compat"
	}
	cfg := map[string]any{
		"default_profile": "default",
		"profiles":        map[string]any{"default": profile},
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

type compatCase struct {
	name string
	args []string
	// host, when set, is exported as URLBOX_API_HOST for the case so
	// network-touching commands hit a controlled endpoint instead of the
	// real API.
	host string
	// exemptStdout / exemptStderr skip the byte-identity assertion for that
	// stream when the output is legitimately state-dependent (e.g. it embeds
	// the config path, which lives under the per-state XDG dir).
	exemptStdout bool
	exemptStderr bool
}

func compatCases() []compatCase {
	return []compatCase{
		{name: "render dry-run", args: []string{"render", "https://example.com", "--dry-run", "--output-format", "json"}},
		{name: "screenshot dry-run", args: []string{"screenshot", "https://example.com", "--dry-run", "--output-format", "json"}},
		{name: "pdf dry-run", args: []string{"pdf", "https://example.com", "--dry-run", "--output-format", "json"}},
		{name: "render curl", args: []string{"render", "https://example.com", "--curl", "--output-format", "json"}},
		{name: "link", args: []string{"link", "https://example.com", "--output-format", "json"}},
		{name: "config get secret", args: []string{"config", "get", "api_secret", "--output-format", "json"}},
		{name: "config path", args: []string{"config", "path", "--output-format", "quiet"}, exemptStdout: true},
		{name: "config profile list", args: []string{"config", "profile", "list", "--output-format", "json"}},
		{name: "schema", args: []string{"schema", "render", "--output-format", "json"}},
		{name: "commands", args: []string{"commands", "--output-format", "json"}},
		{name: "version", args: []string{"version"}},
		// status reaches the network; --no-retry keeps the connection-refused
		// failure immediate so both states fail identically and fast.
		{name: "status", args: []string{"status", "ps_abc123", "--no-retry", "--output-format", "json"}, host: unreachableHost},
		// doctor's config_file check echoes config.Path(), which lives under
		// the per-state XDG dir, so its stdout is legitimately state-dependent.
		{name: "doctor", args: []string{"doctor", "--output-format", "json"}, host: unreachableHost, exemptStdout: true},
		// dashboard in json mode emits a fixed URL envelope and launches no
		// browser — no network, no path, so it must be byte-identical.
		{name: "dashboard", args: []string{"dashboard", "--output-format", "json"}},
		// auth was removed in Plan 2 (login replaces it); it now resolves as
		// an unknown command before any session load, so both states emit the
		// identical routing error.
		{name: "auth removed", args: []string{"auth", "--output-format", "json"}},
	}
}

func runCompat(t *testing.T, args []string) (stdoutStr, stderrStr string, code int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code = Execute(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func TestSessionFieldsDoNotChangeExistingCommands(t *testing.T) {
	for _, tc := range compatCases() {
		t.Run(tc.name, func(t *testing.T) {
			if tc.host != "" {
				t.Setenv("URLBOX_API_HOST", tc.host)
			}

			legacyDir := t.TempDir()
			writeCompatConfig(t, legacyDir, false)
			t.Setenv("XDG_CONFIG_HOME", legacyDir)
			legacyOut, legacyErr, legacyCode := runCompat(t, tc.args)

			sessionDir := t.TempDir()
			writeCompatConfig(t, sessionDir, true)
			t.Setenv("XDG_CONFIG_HOME", sessionDir)
			sessionOut, sessionErr, sessionCode := runCompat(t, tc.args)

			if legacyCode != sessionCode {
				t.Fatalf("exit code changed: legacy=%d session=%d\nlegacy stderr: %s\nsession stderr: %s",
					legacyCode, sessionCode, legacyErr, sessionErr)
			}
			if !tc.exemptStdout && legacyOut != sessionOut {
				t.Fatalf("stdout changed:\nlegacy:  %s\nsession: %s", legacyOut, sessionOut)
			}
			if !tc.exemptStderr && legacyErr != sessionErr {
				t.Fatalf("stderr changed:\nlegacy:  %s\nsession: %s", legacyErr, sessionErr)
			}
		})
	}
}

func TestConfigSetPreservesSessionFields(t *testing.T) {
	dir := t.TempDir()
	writeCompatConfig(t, dir, true)
	t.Setenv("XDG_CONFIG_HOME", dir)
	_, stderr, code := runCompat(t, []string{"config", "set", "api_host", "https://api.urlbox.com"})
	if code != 0 {
		t.Fatalf("config set failed (%d): %s", code, stderr)
	}
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
	if cfg.Profiles["default"]["session_token"] != "sess_tok_compat_123456" {
		t.Fatalf("config set dropped session_token: %s", b)
	}
}

// The Plan-2 credential groups (storage / proxies / llm) require a session and
// an active org, so their output is legitimately session-state-dependent — the
// two-state byte-identity suite does not apply. This pins the divergence
// instead: the legacy (logged-out) state must fail at the auth gate with the
// unified not-logged-in error (exit 3), and the post-login state must get past
// the gate and fail only at the network boundary of the unreachable host
// (exit 11). Any regression that leaks a session requirement into the legacy
// path, or short-circuits the network for a logged-in caller, breaks one arm.
// --no-retry keeps the logged-in arm's connection-refused failure to a single
// attempt so the suite doesn't spend the default retry budget per group.
func TestCredentialGroupsGuardBySessionState(t *testing.T) {
	groups := []struct {
		name string
		args []string
	}{
		{name: "storage list", args: []string{"storage", "list", "--no-retry", "--output-format", "json"}},
		{name: "proxies list", args: []string{"proxies", "list", "--no-retry", "--output-format", "json"}},
		{name: "llm list", args: []string{"llm", "list", "--no-retry", "--output-format", "json"}},
	}
	for _, g := range groups {
		t.Run(g.name, func(t *testing.T) {
			t.Setenv("URLBOX_API_HOST", unreachableHost)

			legacyDir := t.TempDir()
			writeCompatConfig(t, legacyDir, false)
			t.Setenv("XDG_CONFIG_HOME", legacyDir)
			legacyOut, _, legacyCode := runCompat(t, g.args)
			if legacyCode != 3 {
				t.Fatalf("legacy exit code = %d, want 3 (auth)\nstdout: %s", legacyCode, legacyOut)
			}
			if !bytes.Contains([]byte(legacyOut), []byte(notLoggedInMsg)) {
				t.Fatalf("legacy output missing not-logged-in message:\n%s", legacyOut)
			}
			if !bytes.Contains([]byte(legacyOut), []byte(`"code": "auth"`)) {
				t.Fatalf("legacy output missing auth code:\n%s", legacyOut)
			}

			sessionDir := t.TempDir()
			writeCompatConfig(t, sessionDir, true)
			t.Setenv("XDG_CONFIG_HOME", sessionDir)
			sessionOut, _, sessionCode := runCompat(t, g.args)
			if sessionCode != 11 {
				t.Fatalf("session exit code = %d, want 11 (network)\nstdout: %s", sessionCode, sessionOut)
			}
			if !bytes.Contains([]byte(sessionOut), []byte(`"code": "network"`)) {
				t.Fatalf("session output missing network code:\n%s", sessionOut)
			}
			if bytes.Contains([]byte(sessionOut), []byte(notLoggedInMsg)) {
				t.Fatalf("session output must clear the auth gate, still shows not-logged-in:\n%s", sessionOut)
			}
		})
	}
}
