package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

// TestVersion_Subcommand_EmitsEnvelope pins Round 8 II: the new
// `version` subcommand emits an envelope, unlike --version which is
// plain text only. JSON consumers (agents) can call this.
func TestVersion_Subcommand_EmitsEnvelope(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"version", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if env["ok"] != true {
		t.Errorf("ok=%v, want true", env["ok"])
	}
	if env["command"] != "version" {
		t.Errorf("command=%v, want version", env["command"])
	}
	data, _ := env["data"].(map[string]any)
	if _, ok := data["version"]; !ok {
		t.Errorf("data.version missing: %v", data)
	}
	if _, ok := data["commit"]; !ok {
		t.Errorf("data.commit missing: %v", data)
	}
}

func TestVersion_Subcommand_QuietPrintsScalar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"version", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	// Quiet should print just the version string (no JSON, no envelope).
	out := strings.TrimSpace(stdout.String())
	if strings.HasPrefix(out, "{") {
		t.Errorf("quiet mode emitted JSON: %s", out)
	}
}

// TestRootError_CommandFieldNotEmpty pins Round 8 II / Adv-3 H2:
// root-level errors (unknown command, unknown flag, bare `urlbox`)
// must carry a non-empty `command` field. EE fixed sub-subcommand
// path; this fixes the root-level case.
func TestRootError_CommandFieldNotEmpty_UnknownCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"nonexistent_command", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("unknown command should error")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["command"] == "" || env["command"] == nil {
		t.Errorf("command=%v, want non-empty (e.g. \"urlbox\")", env["command"])
	}
}

func TestRootError_CommandFieldNotEmpty_UnknownFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--bogus-root-flag", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatalf("unknown root flag should error")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["command"] == "" || env["command"] == nil {
		t.Errorf("command=%v, want non-empty", env["command"])
	}
}

// TestProfileFlag_EmptyOrWhitespace_Rejected pins Round 8 NN: an
// empty or whitespace-only --profile value used to silently behave
// like "no flag" because the resolver's `if flagProfile != ""` check
// skipped them. Confusing for agents that programmatically set the
// flag from a variable that's sometimes empty. Now rejects loudly.
func TestProfileFlag_EmptyOrWhitespace_Rejected(t *testing.T) {
	cases := []string{"", " ", "   ", "\t", "\n"}
	for _, p := range cases {
		t.Run("profile="+p, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			var stdout, stderr bytes.Buffer
			exit := cmd.Execute([]string{"--profile", p, "config", "profile", "list", "--output-format", "json"}, &stdout, &stderr)
			if exit == 0 {
				t.Fatalf("empty/whitespace --profile should error; got exit 0")
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			if env["code"] != "usage" {
				t.Errorf("code=%v, want usage", env["code"])
			}
		})
	}
}
