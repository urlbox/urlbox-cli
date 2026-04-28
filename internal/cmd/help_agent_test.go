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
