package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/skills"
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
//
// Sourced from skills.Content (the embedded raw markdown) rather than via
// `cmd.Execute("skill", "show")` because the JSON output path escapes
// literal quotes in the markdown — checks like `"timeout"` (closed-set
// error-code value with quotes) wouldn't survive json-encoding.
func TestSkill_DocumentsRenderSurface(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"skill", "show"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	// Read through Execute to keep the path covered; assert against the
	// raw embedded markdown so quoted-string checks aren't defeated by
	// JSON escaping.
	body := skills.Content
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
		// v0.8.0 surface
		"article",    // new preset
		"upstreamOk", // new envelope field
		"--timeout",  // new flag
		`"timeout"`,  // new closed-set error code value
		// v0.8.0 agent-discovery additions (post-audit field-report fixes):
		// the SKILL must teach the --json fallback prominently so agents
		// don't bounce off "no flag for that field" walls.
		"video_scroll",                // sample option only reachable via --json
		"Field not exposed by a flag", // section header — agents read top-down
		"urlbox schema render",        // discovery pointer for the full key set
		"Decision tree for agents",    // explicit branching guidance
	}
	for _, s := range required {
		if !strings.Contains(body, s) {
			t.Errorf("SKILL.md missing %q (regression: a section was removed silently)", s)
		}
	}
}
