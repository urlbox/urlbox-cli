package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestHelpAgent_Root_OutputsJSONEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--help", "--agent"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Fatalf("ok != true: %v", env)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or wrong shape: %v", env)
	}
	if data["name"] != "urlbox" {
		t.Fatalf("name=%v", data["name"])
	}
	flags, _ := data["flags"].([]any)
	if len(flags) == 0 {
		t.Fatalf("flags missing: %v", data)
	}
}

func TestHelpAgent_Subcommand_OutputsJSONEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"commands", "--help", "--agent"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not valid JSON: %v\nstdout: %s", err, stdout.String())
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing")
	}
	if data["name"] != "urlbox commands" {
		t.Fatalf("name=%v", data["name"])
	}
}

func TestHelpAgent_NotSet_FallsThroughToDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	out := stdout.String()
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected plain help, got JSON: %s", out)
	}
}

// v1.0.4 Class 3.3 — --output-format json --help triggers agent help.
//
// Pre-1.0.4 only --agent --help produced JSON; --output-format json
// --help silently fell through to plain text. Agents probing the
// obvious combo (machine-readable help via the existing format flag)
// hit a wall. Now --output-format=json on --help is the equivalent
// trigger.

func TestHelpAgent_OutputFormatJSON_TriggersAgentHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--output-format", "json", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("--output-format json --help must return JSON: %v\nstdout: %s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Fatalf("ok != true: %v", env)
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing: %v", env)
	}
	if data["name"] != "urlbox" {
		t.Fatalf("name=%v", data["name"])
	}
}

func TestHelpAgent_OutputFormatJSON_OnSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "--output-format", "json", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("subcommand --output-format json --help must return JSON: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if data == nil {
		t.Fatalf("expected agent-help shape, got %v", env)
	}
}

// Text mode --help (default) keeps plain text — don't regress the
// human-friendly default.
func TestHelpAgent_OutputFormatText_StaysPlainHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--output-format", "text", "--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") {
		t.Fatalf("text --help must NOT return JSON; got %q", stdout.String())
	}
}
