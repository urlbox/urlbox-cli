package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestSkill_Show_OutputsContent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"skill", "show"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Urlbox CLI Skill") {
		t.Fatalf("expected skill header; got: %s", stdout.String())
	}
}

func TestSkill_Show_JSON_WrapsInEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"skill", "show", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	data, _ := env["data"].(map[string]any)
	skill, _ := data["skill"].(string)
	if skill == "" {
		t.Fatalf("skill field missing or empty: %v", data)
	}
	if !strings.Contains(skill, "Urlbox CLI Skill") {
		t.Fatalf("unexpected skill content: %s", skill)
	}
}

func TestSkill_NoArgs_ShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"skill"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
}
