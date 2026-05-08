package cmd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	e2eBinary   string
	e2eBuildErr error
	e2eBuildDir string
	e2eBuild    sync.Once
)

// getE2EBinary builds the urlbox binary once (shared across all e2e tests) and returns its path.
func getE2EBinary(t *testing.T) string {
	t.Helper()

	if os.Getenv("CI") != "" {
		t.Skip("skipping e2e test in CI (covered by smoke tests)")
	}

	e2eBuild.Do(func() {
		e2eBuildDir, e2eBuildErr = os.MkdirTemp("", "urlbox-e2e-*")
		if e2eBuildErr != nil {
			return
		}

		e2eBinary = filepath.Join(e2eBuildDir, "urlbox")

		//nolint:gosec // test builds the project binary with known-safe arguments
		build := exec.CommandContext(context.Background(), "go", "build", "-o", e2eBinary, "./cmd/urlbox")
		build.Dir = findRepoRoot(t)
		var out []byte
		out, e2eBuildErr = build.CombinedOutput()
		if e2eBuildErr != nil {
			e2eBuildErr = &buildError{err: e2eBuildErr, output: string(out)}
		}
	})

	if e2eBuildErr != nil {
		t.Fatalf("e2e binary build failed: %v", e2eBuildErr)
	}
	t.Cleanup(func() {
		// no-op: dir is shared across tests, cleaned up by OS
	})

	return e2eBinary
}

type buildError struct {
	err    error
	output string
}

func (e *buildError) Error() string {
	return e.err.Error() + "\n" + e.output
}

// ANSI color codes for test output.
const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
)

// runCLI runs the urlbox binary with the given args and returns stdout, stderr, and exit code.
// Logs the command, exit code, and output via t.Log (visible with -v or on failure).
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	bin := getE2EBinary(t)

	//nolint:gosec // test executes the binary it just built with test-controlled args
	cmd := exec.CommandContext(context.Background(), bin, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	if testing.Verbose() {
		fmt.Fprintf(os.Stderr, "  $ urlbox %s\n", strings.Join(args, " "))
	}

	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("failed to run urlbox %v: %v", args, err)
	}

	stdout = outBuf.String()
	stderr = errBuf.String()

	if testing.Verbose() {
		exitColor := colorGreen
		if exitCode != 0 {
			exitColor = colorYellow
		}
		fmt.Fprintf(os.Stderr, "  %sexit=%d%s\n", exitColor, exitCode, colorReset)

		if stdout != "" {
			fmt.Fprintf(os.Stderr, "  stdout:\n%s%s%s\n", colorDim, strings.TrimRight(stdout, "\n"), colorReset)
		}
		if stderr != "" {
			fmt.Fprintf(os.Stderr, "  %sstderr:%s\n%s%s%s\n", colorRed, colorReset, colorDim, strings.TrimRight(stderr, "\n"), colorReset)
		}
	}

	return stdout, stderr, exitCode
}

// --- Commands: JSON format ---

func TestE2E_Commands_JSON_ValidEnvelope(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "commands", "--output-format", "json")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout)
	}

	for _, key := range []string{"ok", "command", "data", "summary"} {
		if _, ok := env[key]; !ok {
			t.Errorf("missing envelope field %q", key)
		}
	}
}

func TestE2E_Commands_JSON_ContainsAllCommands(t *testing.T) {
	stdout, _, _ := runCLI(t, "commands", "--output-format", "json")

	var env struct {
		Data struct {
			Commands []struct {
				Name string `json:"name"`
			} `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	names := make(map[string]bool)
	for _, c := range env.Data.Commands {
		names[c.Name] = true
	}

	for _, want := range []string{"commands", "upgrade"} {
		if !names[want] {
			t.Errorf("expected command %q in catalog, got %v", want, names)
		}
	}
}

func TestE2E_Commands_JSON_HasGlobalFlags(t *testing.T) {
	stdout, _, _ := runCLI(t, "commands", "--output-format", "json")

	var env struct {
		Data struct {
			GlobalFlags []struct {
				Name string `json:"name"`
			} `json:"global_flags"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	found := false
	for _, f := range env.Data.GlobalFlags {
		if f.Name == "output-format" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected output-format in global flags")
	}
}

// --- Commands: text format ---

func TestE2E_Commands_Text_HumanReadable(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "commands", "--output-format", "text")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	if !strings.Contains(stdout, "upgrade") {
		t.Errorf("text output should mention 'upgrade', got %q", stdout)
	}
	if !strings.Contains(stdout, "commands") {
		t.Errorf("text output should mention 'commands', got %q", stdout)
	}
	if !strings.Contains(stdout, "Available commands") {
		t.Errorf("text output should have header, got %q", stdout)
	}
}

// --- Commands: quiet format ---

func TestE2E_Commands_Quiet_DataOnly(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "commands", "--output-format", "quiet")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	if strings.Contains(stdout, `"ok"`) {
		t.Error("quiet output should not contain 'ok' field")
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &data); err != nil {
		t.Fatalf("quiet output is not valid JSON: %v\nOutput: %s", err, stdout)
	}
	if _, ok := data["commands"]; !ok {
		t.Error("expected 'commands' key in quiet data")
	}
}

// --- Commands: piped defaults to JSON ---

func TestE2E_Commands_Piped_DefaultsToJSON(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "commands")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("piped output should be JSON, got: %s", stdout)
	}
	if env["ok"] != true {
		t.Errorf("expected ok=true, got %v", env["ok"])
	}
}

// --- Error envelopes ---

func TestE2E_UnknownCommand_ErrorEnvelope_JSON(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--output-format", "json", "nonexistent")

	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("error output should be JSON: %v\nOutput: %s", err, stdout)
	}
	if env["ok"] != false {
		t.Errorf("expected ok=false, got %v", env["ok"])
	}
	if env["code"] != "usage" {
		t.Errorf("expected code=usage, got %v", env["code"])
	}
	errMsg, _ := env["error"].(string)
	if !strings.Contains(errMsg, "nonexistent") {
		t.Errorf("expected error to mention 'nonexistent', got %q", errMsg)
	}
}

func TestE2E_UnknownCommand_ErrorEnvelope_Text(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--output-format", "text", "nonexistent")

	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d", exitCode)
	}

	if !strings.Contains(stdout, "nonexistent") {
		t.Errorf("expected error to mention 'nonexistent', got %q", stdout)
	}
	if !strings.Contains(stdout, "Error") {
		t.Errorf("expected 'Error' prefix in text output, got %q", stdout)
	}
}

func TestE2E_UnknownCommand_Piped_DefaultsToJSON(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "nonexistent")

	if exitCode != 1 {
		t.Fatalf("expected exit 1, got %d", exitCode)
	}

	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("piped error should be JSON: %v\nOutput: %s", err, stdout)
	}
	if env["ok"] != false {
		t.Errorf("expected ok=false, got %v", env["ok"])
	}
}

// --- Help & version ---

func TestE2E_Help_ExitZero(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--help")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "urlbox") {
		t.Errorf("help should mention 'urlbox', got %q", stdout)
	}
	if !strings.Contains(stdout, "Usage") && !strings.Contains(stdout, "usage") {
		t.Errorf("help should contain usage info, got %q", stdout)
	}
}

func TestE2E_Version_ExitZero(t *testing.T) {
	stdout, _, exitCode := runCLI(t, "--version")

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "commit:") {
		t.Errorf("version should contain 'commit:', got %q", stdout)
	}
}

func TestE2E_NoArgs_ShowsHelp(t *testing.T) {
	stdout, _, exitCode := runCLI(t)

	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "urlbox") {
		t.Errorf("no-args should show help with 'urlbox', got %q", stdout)
	}
}

// --- Output-format flag ---

func TestE2E_OutputFormatFlag_InvalidValue_RejectedExplicitly(t *testing.T) {
	// Pre-v1.0 silently fell through to JSON. v1.0 rejects unknown values
	// up-front so the user/agent gets a clear "use one of: json, text,
	// quiet" error instead of confusion when "yaml" produces JSON.
	stdout, _, exitCode := runCLI(t, "--output-format", "yaml", "commands")

	if exitCode != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", exitCode)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("error envelope should be JSON: %v\nOutput: %s", err, stdout)
	}
	if env["code"] != "usage" {
		t.Errorf("code != 'usage': %v", env["code"])
	}
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "yaml") {
		t.Errorf("error should name the bad value 'yaml'; got: %v", env["error"])
	}
}

func TestE2E_OutputFormatFlag_NDJSON_NotYetImplemented(t *testing.T) {
	// ndjson is reserved for v1.1+ batch streaming. Reject explicitly so
	// users don't think they're getting NDJSON when they're getting JSON.
	stdout, _, exitCode := runCLI(t, "--output-format", "ndjson", "commands")

	if exitCode != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", exitCode)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("error envelope should be JSON: %v\nOutput: %s", err, stdout)
	}
	if env["code"] != "usage" {
		t.Errorf("code != 'usage': %v", env["code"])
	}
	if errMsg, _ := env["error"].(string); !strings.Contains(errMsg, "ndjson") {
		t.Errorf("error should mention 'ndjson'; got: %v", env["error"])
	}
}

// --- Exit codes ---

func TestE2E_ExitCodes(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantExit int
	}{
		{"help", []string{"--help"}, 0},
		{"version", []string{"--version"}, 0},
		{"commands", []string{"commands"}, 0},
		{"unknown command", []string{"nonexistent"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, exitCode := runCLI(t, tt.args...)
			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d", exitCode, tt.wantExit)
			}
		})
	}
}
