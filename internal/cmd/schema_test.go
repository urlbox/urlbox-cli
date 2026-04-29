// internal/cmd/schema_test.go
package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestSchemaRender_DefaultJSON_HasEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"schema", "render", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Errorf("ok != true: %v", env["ok"])
	}
	if env["command"] != "schema render" {
		t.Errorf("command != 'schema render': %v", env["command"])
	}
	if env["summary"] != "Render JSON Schema (Draft 2020-12)" {
		t.Errorf("summary mismatch: %v", env["summary"])
	}

	bcs, ok := env["breadcrumbs"].([]any)
	if !ok || len(bcs) == 0 {
		t.Fatalf("breadcrumbs missing or empty: %v", env["breadcrumbs"])
	}
	bc := bcs[0].(map[string]any)
	if bc["action"] != "render" {
		t.Errorf("breadcrumb[0].action != 'render': %v", bc["action"])
	}
	if !strings.Contains(bc["cmd"].(string), "urlbox render --json") {
		t.Errorf("breadcrumb[0].cmd unexpected: %v", bc["cmd"])
	}

	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %T", env["data"])
	}
	props, ok := data["properties"].(map[string]any)
	if !ok {
		t.Fatalf("data.properties is not an object")
	}
	if _, ok := props["url"]; !ok {
		t.Errorf("schema missing properties.url")
	}
}

func TestSchemaRender_Quiet_RawSchemaOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"schema", "render", "--output-format", "quiet"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("not JSON: %v\nstdout=%s", err, stdout.String())
	}
	// Quiet must NOT have the envelope keys.
	for _, k := range []string{"ok", "command", "summary", "breadcrumbs"} {
		if _, present := raw[k]; present {
			t.Errorf("--output-format quiet leaked envelope key %q", k)
		}
	}
	// Raw schema is present.
	if _, ok := raw["properties"]; !ok {
		t.Errorf("quiet output missing 'properties'")
	}
}

func TestSchemaRender_JQ_FiltersInsideEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute(
		[]string{"schema", "render", "--output-format", "json", "--jq", ".data.properties.url.type"},
		&stdout, &stderr,
	)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	got := strings.TrimSpace(stdout.String())
	// The url field should be a string in the schema. Acceptable forms: "string"
	// (alone) or ["string", "null"] (a union with null). We assert "string" is
	// present in the output.
	if !strings.Contains(got, `"string"`) {
		t.Errorf("expected url type to contain \"string\"; got: %s", got)
	}
}

func TestSchema_NoArgs_ShowsHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"schema"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
}

func TestSchemaRender_HelpAgent_StructuredJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"schema", "render", "--help", "--agent"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("--help --agent did not output JSON: %v\nstdout=%s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Errorf("ok != true: %v", env["ok"])
	}
}
