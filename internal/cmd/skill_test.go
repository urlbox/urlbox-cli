package cmd_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
		// v0.8.1 — agent self-bootstrap: SKILL.md must teach the install
		// flow so an agent reading it knows how to register the skill with
		// the user's tooling.
		"urlbox skill install", // the new command
		"Bootstrap",            // section header at the top
		"--target claude-code", // the canonical target invocation
		// v0.9.0 — schema-as-docs: SKILL.md must teach the new validation
		// contract so agents understand --json is a passthrough and that
		// the API (not the CLI) is the authoritative validator.
		"Validation contract", // new section header (replaces old "Validation")
		"passthrough",         // load-bearing word for the --json contract
		"warning:",            // stderr prefix the warning system uses
		// Phase 5 — link / status / dashboard surface. These commands ship
		// agent-relevant behaviour (URL signing, async polling, headless
		// fallback) so the skill must mention them or the agent is blind.
		"urlbox link",      // signed URL command
		"urlbox status",    // async-render polling command
		"urlbox dashboard", // browser-open command
		"--wait",           // status polling flag
		"--poll-interval",  // status polling cadence flag
		"renderId",         // status command's positional arg term
		"HMAC",             // load-bearing crypto term for link
		"headless",         // dashboard fallback behaviour
	}
	for _, s := range required {
		if !strings.Contains(body, s) {
			t.Errorf("SKILL.md missing %q (regression: a section was removed silently)", s)
		}
	}
}

// TestSkillInstall_NonTTY_Errors confirms an agent calling skill install
// without --target gets a clear error pointing at the supported set, rather
// than hanging on a non-existent prompt. Mirrors the Fizzy CLI gap that
// motivated this command.
func TestSkillInstall_NonTTY_NoTarget_Errors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"skill", "install", "--yes", "--output-format", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit when --target missing without TTY")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, "claude-code") {
		t.Errorf("hint should list supported targets; got %q", hint)
	}
}

func TestSkillInstall_UnsupportedTarget_Errors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"skill", "install",
		"--target", "vim",
		"--scope", "user",
		"--yes",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on unsupported target")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if env["code"] != "usage" {
		t.Errorf("code=%v, want usage", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "vim") {
		t.Errorf("error should name the bad target %q; got %q", "vim", errMsg)
	}
}

func TestSkillInstall_BadScope_Errors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"skill", "install",
		"--target", "claude-code",
		"--scope", "global",
		"--yes",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on invalid scope")
	}
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, "user") || !strings.Contains(hint, "project") {
		t.Errorf("hint should list valid scopes (user/project); got %q", hint)
	}
}

func TestSkillInstall_ClaudeCode_User_WritesFileAtHomePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"skill", "install",
		"--target", "claude-code",
		"--scope", "user",
		"--yes",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	wantPath := filepath.Join(tmp, ".claude", "skills", "urlbox", "SKILL.md")
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if !strings.Contains(string(body), "Urlbox CLI Skill") {
		t.Fatalf("file content unexpected; first 80 bytes: %q", string(body[:min(80, len(body))]))
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data, _ := env["data"].(map[string]any)
	if data["target"] != "claude-code" {
		t.Errorf("data.target=%v, want claude-code", data["target"])
	}
	if data["scope"] != "user" {
		t.Errorf("data.scope=%v, want user", data["scope"])
	}
	if data["path"] != wantPath {
		t.Errorf("data.path=%v, want %q", data["path"], wantPath)
	}
}

func TestSkillInstall_ClaudeCode_Project_WritesUnderCWD(t *testing.T) {
	tmp := t.TempDir()
	origCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origCWD) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{
		"skill", "install",
		"--target", "claude-code",
		"--scope", "project",
		"--yes",
		"--output-format", "json",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	wantPath := filepath.Join(tmp, ".claude", "skills", "urlbox", "SKILL.md")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("project-scope file not written at %s: %v", wantPath, err)
	}
}
