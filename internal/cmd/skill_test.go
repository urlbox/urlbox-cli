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

// Regression guard: SKILL.md must mention every render-adjacent command and
// every render-related flag the agent needs to know about. If a future
// commit removes a section silently, this test catches it.
func TestSkill_DocumentsRenderSurface(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"skill", "show"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	body := stdout.String()
	required := []string{
		// Command surface
		"urlbox render",
		"urlbox screenshot",
		"urlbox pdf",
		"urlbox video",
		// Flags / modes
		"--json",
		"--dry-run",
		"--curl",
		"--output",
		"--open",
		"--preset",
		"--async",
		// Reliability
		"retry",
		// Closed error-code set
		"validation",
		"rate_limit",
		"network",
	}
	for _, s := range required {
		if !strings.Contains(body, s) {
			t.Errorf("SKILL.md missing %q (regression: a section was removed silently)", s)
		}
	}
}
