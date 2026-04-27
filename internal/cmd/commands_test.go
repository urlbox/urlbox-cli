// internal/cmd/commands_test.go
package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestCommands_JSON_ReturnsValidEnvelope(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout.String())
	}
	if !env.OK {
		t.Error("expected ok=true")
	}
	if env.Command != "commands" {
		t.Errorf("command = %q, want %q", env.Command, "commands")
	}
}

func TestCommands_JSON_ContainsUpgradeCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)
	out := stdout.String()
	if !strings.Contains(out, "upgrade") {
		t.Errorf("expected catalog to contain 'upgrade', got %s", out)
	}
}

func TestCommands_JSON_ContainsCommandsItself(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)
	out := stdout.String()
	if !strings.Contains(out, `"commands"`) {
		t.Errorf("expected catalog to contain 'commands', got %s", out)
	}
}

func TestCommands_JSON_HasCatalogStructure(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)
	var env struct {
		OK      bool   `json:"ok"`
		Command string `json:"command"`
		Data    struct {
			Commands    []json.RawMessage `json:"commands"`
			GlobalFlags []json.RawMessage `json:"global_flags"`
		} `json:"data"`
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error: %v\nOutput: %s", err, stdout.String())
	}
	if len(env.Data.Commands) == 0 {
		t.Error("expected at least one command in catalog")
	}
	if len(env.Data.GlobalFlags) == 0 {
		t.Error("expected at least one global flag in catalog")
	}
	if env.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestCommands_JSON_CommandHasNameAndDescription(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)
	var env struct {
		Data struct {
			Commands []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	for _, c := range env.Data.Commands {
		if c.Name == "" {
			t.Error("found command with empty name")
		}
		if c.Description == "" {
			t.Errorf("command %q has empty description", c.Name)
		}
	}
}

func TestCommands_JSON_FlagsIncluded(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Execute([]string{"commands", "--output-format", "json"}, stdout, stderr)
	var env struct {
		Data struct {
			GlobalFlags []struct {
				Name        string `json:"name"`
				Type        string `json:"type"`
				Description string `json:"description"`
			} `json:"global_flags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	found := false
	for _, f := range env.Data.GlobalFlags {
		if f.Name == "output-format" {
			found = true
			if f.Type == "" {
				t.Error("output-format flag missing type")
			}
			break
		}
	}
	if !found {
		t.Error("expected output-format in global flags")
	}
}

func TestCommands_ExitCodeZero(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"commands"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", code, stderr.String())
	}
}

func TestCommands_TextMode_HumanReadable(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"commands", "--output-format", "text"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "upgrade") {
		t.Errorf("text output should mention 'upgrade', got %q", out)
	}
	if !strings.Contains(out, "commands") {
		t.Errorf("text output should mention 'commands', got %q", out)
	}
}

func TestCommands_QuietMode_DataOnly(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	code := cmd.Execute([]string{"commands", "--output-format", "quiet"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	out := stdout.String()
	if strings.Contains(out, `"ok"`) {
		t.Error("quiet output should not contain envelope 'ok' field")
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &data); err != nil {
		t.Fatalf("quiet output is not valid JSON: %v\nOutput: %s", err, out)
	}
	if _, ok := data["commands"]; !ok {
		t.Error("expected 'commands' key in quiet data output")
	}
}
