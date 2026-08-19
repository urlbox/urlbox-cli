package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestRoot_HasProfileFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--help"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	help := stdout.String() + stderr.String()
	if !strings.Contains(help, "--profile") {
		t.Error("--help is missing --profile")
	}
	if strings.Contains(help, "--env") {
		t.Error("--help contains --env; the flag was dropped in Phase 3")
	}
}

func TestRootCommand_VersionFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--version"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox") {
		t.Errorf("expected version output to contain 'urlbox', got %q", out)
	}
}

func TestRootCommand_Help(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox") {
		t.Errorf("expected help output to contain 'urlbox', got %q", out)
	}
	if !strings.Contains(out, "Usage") && !strings.Contains(out, "usage") {
		t.Errorf("expected help output to contain usage info, got %q", out)
	}
}

func TestRootCommand_UnknownSubcommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"nonexistent"}, stdout, stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown subcommand")
	}
	// Error should be in the envelope on stdout (not stderr)
	out := stdout.String()
	if !strings.Contains(out, "unknown") || !strings.Contains(out, "nonexistent") {
		t.Errorf("expected error about unknown command on stdout, got %q", out)
	}
}

// TestAuthCommandStillRegistered guards the headless bootstrap path.
// `urlbox login` needs a browser, so `urlbox auth` remains the only
// non-interactive way to write a secret into a fresh config on a machine
// that has never been logged in. Removing it regresses CI and agent setup.
func TestAuthCommandStillRegistered(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"auth"}, stdout, stderr)
	out := stdout.String()
	if strings.Contains(out, "unknown command") {
		t.Fatalf("`urlbox auth` must stay registered as the headless bootstrap path; got %q", out)
	}
	// No secret supplied and no TTY, so it fails on input — not on the
	// command being missing. Exit code is usage, not the unknown-command path.
	if code == 0 {
		t.Errorf("expected a usage failure with no secret supplied, got exit 0: %q", out)
	}
}

func TestRootCommand_NoArgs_ShowsHelp(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox") {
		t.Errorf("expected help output to contain 'urlbox', got %q", out)
	}
}

func TestRootCommand_VersionFormat(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--version"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "commit:") {
		t.Errorf("expected version to contain 'commit:', got %q", out)
	}
	if !strings.Contains(out, "built:") {
		t.Errorf("expected version to contain 'built:', got %q", out)
	}
}

func TestRootCommand_HelpGoesToStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"--help"}, stdout, stderr)
	if stdout.Len() == 0 {
		t.Error("expected help output on stdout, got nothing")
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no output on stderr for --help, got %q", stderr.String())
	}
}

func TestRootCommand_ErrorGoesToStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"nonexistent"}, stdout, stderr)
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}
	if stdout.Len() == 0 {
		t.Error("expected error output on stdout, got nothing")
	}
}

func TestRootCommand_OutputFormatFlag_Accepted(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--output-format", "json", "--help"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
}

func TestRootCommand_UnknownSubcommand_JSONErrorEnvelope(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"--output-format", "json", "nonexistent"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", code)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if env["ok"] != false {
		t.Errorf("expected ok=false, got %v", env["ok"])
	}
	if env["code"] != "usage" {
		t.Errorf("expected code=usage, got %v", env["code"])
	}
}

func TestRootCommand_UnknownSubcommand_ExitCode1(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"nonexistent"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected exit code 1 (usage), got %d", code)
	}
}

func TestRootCommand_ErrorEnvelope_GoesToStdout(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"--output-format", "json", "nonexistent"}, stdout, stderr)
	if stdout.Len() == 0 {
		t.Error("expected error envelope on stdout, got nothing")
	}
}

func TestRoot_NoArgs_PrintsBannerToStderr_WhenTTY(t *testing.T) {
	cmd.SetStderrTTYForTest(true)
	defer cmd.ResetStderrTTYForTest()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{}, stdout, stderr)

	if !strings.Contains(stderr.String(), "urlbox commands") {
		t.Fatalf("expected banner on stderr; got: %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "If you're looking") {
		t.Fatalf("banner must not appear on stdout; got: %q", stdout.String())
	}
}

func TestRoot_NoArgs_NoBanner_WhenPiped(t *testing.T) {
	cmd.SetStderrTTYForTest(false)
	defer cmd.ResetStderrTTYForTest()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{}, stdout, stderr)

	if strings.Contains(stderr.String(), "If you're looking") {
		t.Fatalf("banner should not show in non-TTY; got: %q", stderr.String())
	}
}

func TestRoot_WithSubcommand_NoBanner(t *testing.T) {
	cmd.SetStderrTTYForTest(true)
	defer cmd.ResetStderrTTYForTest()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)

	if strings.Contains(stderr.String(), "If you're looking") {
		t.Fatalf("banner leaked into subcommand run; got: %q", stderr.String())
	}
}

func TestUnknownCommand_SuggestsDidYouMean(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"rendr"}, &stdout, &stderr) // typo of "render"
	if exit == 0 {
		t.Fatal("expected non-zero exit on unknown command")
	}
	combined := stdout.String() + stderr.String()
	// Assert on the literal `Did you mean "render"` (with closing quote)
	// so the test fails if suggestUnknownCommand is silently removed.
	// The default text formatter renders the hint verbatim; the JSON
	// formatter escapes quotes (`Did you mean \"render\"`). Accept either.
	if !strings.Contains(combined, `Did you mean "render"`) &&
		!strings.Contains(combined, `Did you mean \"render\"`) {
		t.Errorf("expected `Did you mean \"render\"` suggestion; got stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
}

func TestUnknownFlag_SuggestsDidYouMean(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"render", "https://example.com", "--output-formart", "json"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on unknown flag")
	}
	combined := stdout.String() + stderr.String()
	// The suggestion logic must actually fire — assert on the literal
	// `Did you mean "--output-format"` (with the closing quote) so this
	// test fails if suggestUnknownFlag is silently removed. Without this,
	// the substring "output-format" would already appear in pflag's raw
	// "unknown flag: --output-formart" error and pass spuriously.
	// The default text formatter renders the hint verbatim; the JSON
	// formatter escapes quotes (`Did you mean \"--output-format\"`).
	// Accept either rendering.
	if !strings.Contains(combined, `Did you mean "--output-format"`) &&
		!strings.Contains(combined, `Did you mean \"--output-format\"`) {
		t.Errorf("expected `Did you mean \"--output-format\"` suggestion; got stdout=%q stderr=%q",
			stdout.String(), stderr.String())
	}
}

func TestUnknownCommand_FarTypo_NoSuggestion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"xyzzy"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on unknown command")
	}
	combined := stdout.String() + stderr.String()
	if !strings.Contains(combined, "xyzzy") {
		t.Errorf("expected error to name unknown command 'xyzzy'; got %q", combined)
	}
}

// TestRoot_DidYouMean_NoSelfSuggestion pins Round 6 Tester #2 + Adv-8:
// the suggester used to return the exact rejected flag back as the
// suggestion, because its candidate pool was the union of every flag
// across every command in the tree. If the typed flag existed on ANY
// command (even an unrelated one), ClosestMatch returned it as "the
// closest match" — the rejected flag itself.
//
// Class-fix: the suggester now scopes its candidate pool to the active
// command's own flag set (plus inherited persistent flags). Flags from
// unrelated commands are no longer fair game. The rejected token is
// also explicitly excluded as a belt-and-braces guard.
func TestRoot_DidYouMean_NoSelfSuggestion(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		// `render` has no --url (it takes positional URL). Without the
		// class-fix, this suggests --url (from link).
		{"render --url tautology", []string{"render", "--url", "https://example.com"}},
		// `link` accepts only api-* + format + jq + json + output-format + profile + url.
		// --width is render's. Used to suggest --width back to itself.
		{"link --width tautology", []string{"link", "--width", "100"}},
		// `doctor` doesn't take --api-secret-file. Used to suggest itself.
		{"doctor --api-secret-file tautology", []string{"doctor", "--api-secret-file", "/tmp/x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("URLBOX_API_SECRET", "sec_test")

			args := append([]string(nil), c.args...)
			args = append(args, "--output-format", "json")
			var stdout, stderr bytes.Buffer
			exit := cmd.Execute(args, &stdout, &stderr)
			if exit == 0 {
				t.Fatalf("expected non-zero exit on unknown flag; got 0. stdout=%s", stdout.String())
			}
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			hint, _ := env["hint"].(string)
			// The flag the user TYPED must not appear in the suggestion. Locate
			// the rejected flag from the args and assert the hint doesn't
			// repeat it back.
			var rejected string
			for _, a := range c.args {
				if strings.HasPrefix(a, "--") {
					rejected = a
					break
				}
			}
			if rejected == "" {
				t.Fatalf("test bug: couldn't find a -- flag in args %v", c.args)
			}
			suggested := `Did you mean "` + rejected + `"`
			if strings.Contains(hint, suggested) {
				t.Errorf("hint contains tautological suggestion %q: %s", suggested, hint)
			}
		})
	}
}

// TestRoot_DidYouMean_TyposStillWork pins that legitimate typos still
// get a useful suggestion — the class-fix narrows the pool, it doesn't
// eliminate suggestions.
func TestRoot_DidYouMean_TyposStillWork(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "sec_test")

	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"render", "--widht", "100", "--output-format", "json"}, &stdout, &stderr)
	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, `--width`) {
		t.Errorf("--widht should suggest --width on render; got hint %q", hint)
	}
	if strings.Contains(hint, `--widht`) {
		t.Errorf("hint contains the typo back: %q", hint)
	}
}

// TestRoot_UnknownSubcommand_ScopedSuggestion pins Round 6 Adv-9 bonus
// finding: `urlbox schema list` suggested "link" — a sibling top-level
// command — because the suggester walked root.Commands() instead of
// the actual parent's subcommands. Class-fix: scope candidates to the
// parent path mentioned in cobra's error string ("for `urlbox schema`").
func TestRoot_UnknownSubcommand_ScopedSuggestion(t *testing.T) {
	cases := []struct {
		name string
		args []string
		// Either "no suggestion" (empty string) OR an expected scoped
		// suggestion. Cross-tree suggestions (like "link" for schema
		// list) must NOT appear.
		mustNotContain []string
	}{
		{"schema list -> no cross-tree link", []string{"schema", "list"}, []string{`"link"`, `"render"` /* render is sibling */}},
		{"config nonsense -> no cross-tree", []string{"config", "nonsense"}, []string{`"render"`, `"link"`, `"doctor"`}},
		{"config profile nonsense -> no cross-tree", []string{"config", "profile", "nonsense"}, []string{`"render"`, `"link"`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			args := append(append([]string(nil), c.args...), "--output-format", "json")
			var stdout, stderr bytes.Buffer
			cmd.Execute(args, &stdout, &stderr)
			var env map[string]any
			_ = json.Unmarshal(stdout.Bytes(), &env)
			hint, _ := env["hint"].(string)
			for _, bad := range c.mustNotContain {
				if strings.Contains(hint, `Did you mean `+bad) {
					t.Errorf("hint cross-suggested %s for %v: %q", bad, c.args, hint)
				}
			}
		})
	}
}

// TestRoot_EnvelopeOkMatchesExit_Contract is the meta-test: every
// command/scenario that produces ok:false in the envelope MUST also
// exit non-zero, and vice versa. Round 6 Adv-9 surfaced sibling
// suspicions on schema list / config profile show; this test walks a
// representative slice of error paths and pins the contract.
func TestRoot_EnvelopeOkMatchesExit_Contract(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"unknown top command", []string{"nonsense"}},
		{"unknown subcommand of schema", []string{"schema", "list"}},
		{"unknown subcommand of config", []string{"config", "nonsense"}},
		{"unknown subcommand of config profile", []string{"config", "profile", "nonsense"}},
		{"unknown subcommand of skill", []string{"skill", "nonsense"}},
		{"unknown flag root-level", []string{"--nonsense"}},
		{"render missing url", []string{"render"}},
		{"status missing renderid", []string{"status"}},
		{"link missing url", []string{"link"}},
		{"config get unknown key", []string{"config", "get", "noKey"}},
		{"config set wrong arity", []string{"config", "set", "api_secret"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("URLBOX_API_SECRET", "sec_test")
			args := append(append([]string(nil), c.args...), "--output-format", "json")
			var stdout, stderr bytes.Buffer
			exit := cmd.Execute(args, &stdout, &stderr)
			var env map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("envelope not JSON: %v\nstdout=%s", err, stdout.String())
			}
			ok, _ := env["ok"].(bool)
			// Contract: ok:false <-> exit != 0
			if ok && exit != 0 {
				t.Errorf("envelope ok=true but exit=%d (non-zero); args=%v stdout=%s", exit, c.args, stdout.String())
			}
			if !ok && exit == 0 {
				t.Errorf("envelope ok=false but exit=0; args=%v stdout=%s", c.args, stdout.String())
			}
		})
	}
}

// ─── v1.0.4 Class 3.2 — error stream routing per format ────────────
//
// Invariant: the output stream a byte goes to depends on what kind of
// byte it is, not on where the code happens to be.
//   - JSON envelope = data (agent contract: parseable structure) → stdout.
//   - Text-mode "Error:" line = human message → stderr.
//   - Quiet-mode error scalar = human message → stderr.
//
// Pre-v1.0.4 root.go Execute() always wrote errors to stdout,
// violating CLAUDE.md "Stdout for data, stderr for human messages.
// Never mix." for text + quiet formats.

func TestExecute_TextError_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--output-format", "text", "nonexistent"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit for unknown command")
	}
	if stdout.Len() > 0 {
		t.Errorf("text-mode error must NOT write to stdout; got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Error:") {
		t.Errorf("text-mode error must write 'Error:' to stderr; stderr=%q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "nonexistent") {
		t.Errorf("text-mode error must mention the bad token; stderr=%q", stderr.String())
	}
}

func TestExecute_QuietError_GoesToStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--output-format", "quiet", "nonexistent"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit")
	}
	if stdout.Len() > 0 {
		t.Errorf("quiet-mode error must NOT write to stdout (pipelines read stdout for data); got %q", stdout.String())
	}
	if stderr.Len() == 0 {
		t.Errorf("quiet-mode error must write error message to stderr")
	}
}

func TestExecute_JSONError_StaysOnStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"--output-format", "json", "nonexistent"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit")
	}
	if stdout.Len() == 0 {
		t.Errorf("json-mode error envelope must be on stdout (agent contract); stderr=%q", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("json-mode error envelope must be valid JSON on stdout; got %q (err=%v)", stdout.String(), err)
	}
	if env["code"] != "usage" {
		t.Errorf("expected code=usage, got %v", env["code"])
	}
}
