package output_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestTextFormatter_FailureUsesErrorGlyph(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("doctor", map[string]any{"status": "fail"}, "Some checks failed", nil)
	env.OK = false
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "✗") {
		t.Errorf("ok=false summary should use ✗, got %q", out)
	}
	if strings.Contains(out, "✓") {
		t.Errorf("ok=false summary must not print ✓, got %q", out)
	}
}

func TestTextFormatter_SuccessUsesCheckGlyph(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("orgs list", nil, "2 organisations", nil)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	if !strings.Contains(buf.String(), "✓") {
		t.Errorf("ok=true summary should use ✓, got %q", buf.String())
	}
}

func TestTextFormatter_RendersTableView(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("orgs list", map[string]any{"n": 2}, "2 organisations", nil)
	env.SetTable([]string{"NAME", "ID"}, [][]string{{"Acme", "org_a"}, {"Globex", "org_b"}}, 1)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Acme", "Globex", "org_a", "●"} {
		if !strings.Contains(out, want) {
			t.Errorf("table text view missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "2 organisations") {
		t.Errorf("ok+table view should omit the summary line, got:\n%s", out)
	}
}

func TestTextFormatter_RendersKVView(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	env := output.NewEnvelope("whoami", map[string]any{"email": "u@x.com"}, "Signed in as u@x.com", nil)
	env.SetKV([][2]string{{"Signed in", "u@x.com"}, {"Org", "Acme (org_a)"}})
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatText, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"u@x.com", "Acme (org_a)"} {
		if !strings.Contains(out, want) {
			t.Errorf("KV text view missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Signed in as u@x.com") {
		t.Errorf("ok+KV view should omit the summary line, got:\n%s", out)
	}
}

func TestTextView_NotSerialisedInJSON(t *testing.T) {
	env := output.NewEnvelope("orgs list", map[string]any{"n": 2}, "2 organisations", nil)
	env.SetTable([]string{"NAME"}, [][]string{{"Acme"}}, -1)
	buf := &bytes.Buffer{}
	f := output.NewFormatter(output.FormatJSON, output.NewStylesForWriter(buf))

	if err := f.WriteSuccess(buf, env); err != nil {
		t.Fatalf("WriteSuccess error: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, forbidden := range []string{"NAME", "Acme", "table", "kv", "textView", "text_view"} {
		if strings.Contains(buf.String(), forbidden) {
			t.Errorf("JSON output leaked text-view content %q:\n%s", forbidden, buf.String())
		}
	}
}
