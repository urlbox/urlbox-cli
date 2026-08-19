// internal/output/format_test.go
package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestJSONFormatter_WriteSuccess_ValidJSON(t *testing.T) {
	env := output.NewEnvelope("test", map[string]string{"key": "val"}, "summary", nil)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatJSON, &output.Styles{})

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}

	var decoded output.Envelope
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}
	if !decoded.OK {
		t.Error("expected ok=true in JSON output")
	}
	if decoded.Command != "test" {
		t.Errorf("command = %q, want %q", decoded.Command, "test")
	}
}

func TestJSONFormatter_WriteError_ValidJSON(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrAuth, "denied", "run auth")
	env := output.NewErrorEnvelope("render", cliErr)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatJSON, &output.Styles{})

	if err := f.WriteError(buf, env); err != nil {
		t.Fatalf("WriteError error: %v", err)
	}

	var decoded output.ErrorEnvelope
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}
	if decoded.OK {
		t.Error("expected ok=false in error JSON")
	}
	if decoded.Code != "auth" {
		t.Errorf("code = %q, want %q", decoded.Code, "auth")
	}
}

func TestTextFormatter_WriteSuccess_ContainsSummary(t *testing.T) {
	env := output.NewEnvelope("test", nil, "Completed OK", nil)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}

	if !strings.Contains(buf.String(), "Completed OK") {
		t.Errorf("expected output to contain summary, got %q", buf.String())
	}
}

func TestTextFormatter_WriteError_ContainsMessage(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrUsage, "missing url", "provide a URL")
	env := output.NewErrorEnvelope("render", cliErr)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteError(buf, env); err != nil {
		t.Fatalf("WriteError error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "missing url") {
		t.Errorf("expected error message in output, got %q", out)
	}
	if !strings.Contains(out, "provide a URL") {
		t.Errorf("expected hint in output, got %q", out)
	}
}

func TestTextFormatter_WriteSuccess_OkWithView_ViewOnlyNoSummary(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("orgs list", map[string]any{"n": 2}, "2 organisations", nil)
	env.SetTable([]string{"NAME", "ID"}, [][]string{{"Acme", "org_a"}}, -1)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "2 organisations") {
		t.Errorf("ok+view text should omit the summary line, got:\n%s", out)
	}
	if !strings.Contains(out, "Acme") || !strings.Contains(out, "org_a") {
		t.Errorf("ok+view text should still render the view, got:\n%s", out)
	}
}

func TestTextFormatter_WriteSuccess_NotOkWithView_SummaryThenView(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("doctor", map[string]any{"status": "fail"}, "Some checks failed", nil)
	env.OK = false
	env.SetTable([]string{"CHECK", "STATUS"}, [][]string{{"api_secret", "fail"}}, -1)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Some checks failed") {
		t.Errorf("not-ok+view text should keep the summary line, got:\n%s", out)
	}
	if !strings.Contains(out, "✗") {
		t.Errorf("not-ok+view summary should use ✗, got:\n%s", out)
	}
	if !strings.Contains(out, "api_secret") {
		t.Errorf("not-ok+view text should render the view, got:\n%s", out)
	}
	summaryIdx := strings.Index(out, "Some checks failed")
	viewIdx := strings.Index(out, "api_secret")
	if summaryIdx > viewIdx {
		t.Errorf("summary must appear above the view, got:\n%s", out)
	}
}

func TestTextFormatter_WriteSuccess_OkNoView_SummaryAsToday(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("logout", nil, "Logged out", nil)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Logged out") {
		t.Errorf("ok+no-view text should keep the summary line, got:\n%s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Errorf("ok+no-view summary should use ✓, got:\n%s", out)
	}
}

func TestQuietFormatter_WriteSuccess_DataOnly(t *testing.T) {
	env := output.NewEnvelope("test", map[string]string{"id": "abc"}, "summary", nil)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatQuiet, &output.Styles{})

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}

	out := buf.String()
	var data map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &data); err != nil {
		t.Fatalf("quiet output is not valid data JSON: %v\nOutput: %s", err, out)
	}
	if data["id"] != "abc" {
		t.Errorf("data[id] = %q, want %q", data["id"], "abc")
	}
	if strings.Contains(out, `"ok"`) {
		t.Error("quiet output should not contain envelope 'ok' field")
	}
}

func TestQuietFormatter_WriteError_MessageOnly(t *testing.T) {
	cliErr := output.NewCLIError(output.ErrServer, "server error", "")
	env := output.NewErrorEnvelope("test", cliErr)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatQuiet, &output.Styles{})

	if err := f.WriteError(buf, env); err != nil {
		t.Fatalf("WriteError error: %v", err)
	}

	out := strings.TrimSpace(buf.String())
	if out != "server error" {
		t.Errorf("quiet error output = %q, want %q", out, "server error")
	}
}

func TestNewFormatter_ReturnsCorrectType(t *testing.T) {
	tests := []struct {
		format output.Format
		name   string
	}{
		{output.FormatJSON, "json"},
		{output.FormatText, "text"},
		{output.FormatQuiet, "quiet"},
		{"unknown", "json"},
	}
	for _, tt := range tests {
		f := output.NewFormatter(tt.format, &output.Styles{})
		if f == nil {
			t.Errorf("NewFormatter(%q) returned nil", tt.format)
		}
	}
}

func TestJSONFormatter_WriteSuccess_NilData(t *testing.T) {
	env := output.NewEnvelope("test", nil, "", nil)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatJSON, &output.Styles{})

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if string(raw["data"]) != "null" {
		t.Errorf("expected data to be null, got %s", raw["data"])
	}
}
