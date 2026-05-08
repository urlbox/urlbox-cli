// internal/output/jq_test.go
package output_test

import (
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestApplyJQ_FilterField(t *testing.T) {
	in := []byte(`{"ok":true,"data":{"render_id":"abc"}}`)
	out, err := output.ApplyJQ(in, ".data.render_id", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != `"abc"` {
		t.Fatalf("got: %q", out)
	}
}

func TestApplyJQ_QuietRunsOnData(t *testing.T) {
	in := []byte(`{"ok":true,"data":{"render_id":"abc"}}`)
	out, err := output.ApplyJQ(in, ".render_id", true)
	if err != nil {
		t.Fatal(err)
	}
	// Quiet mode strips quotes from scalar strings (matches `jq -r`),
	// so the canonical pipeline `--jq .renderId | xargs ...` works.
	if strings.TrimSpace(string(out)) != `abc` {
		t.Fatalf("got: %q", out)
	}
}

func TestApplyJQ_InvalidExprReturnsError(t *testing.T) {
	_, err := output.ApplyJQ([]byte(`{}`), "..[", false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestApplyJQ_MultipleResults(t *testing.T) {
	in := []byte(`{"data":{"items":[1,2,3]}}`)
	out, err := output.ApplyJQ(in, ".data.items[]", false)
	if err != nil {
		t.Fatal(err)
	}
	want := "1\n2\n3"
	if strings.TrimSpace(string(out)) != want {
		t.Fatalf("got: %q want: %q", out, want)
	}
}

func TestApplyJQ_StringTypeOutput(t *testing.T) {
	in := []byte(`{"name":"hello"}`)
	out, err := output.ApplyJQ(in, ".name", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != `"hello"` {
		t.Fatalf("got: %q", out)
	}
}

func TestApplyJQ_QuietMode_ScalarString_EmitsRaw(t *testing.T) {
	body := []byte(`{"ok":true,"command":"render","data":{"renderId":"abc123"}}`)
	out, err := output.ApplyJQ(body, ".renderId", true)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSuffix(string(out), "\n")
	if got != "abc123" {
		t.Errorf("got %q, want %q (no surrounding quotes; matches jq -r)", got, "abc123")
	}
}

func TestApplyJQ_QuietMode_ScalarNumber_StillJSONFormatted(t *testing.T) {
	body := []byte(`{"ok":true,"command":"render","data":{"size":12345}}`)
	out, err := output.ApplyJQ(body, ".size", true)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSuffix(string(out), "\n")
	if got != "12345" {
		t.Errorf("got %q, want %q", got, "12345")
	}
}

func TestApplyJQ_QuietMode_Object_StillJSONFormatted(t *testing.T) {
	body := []byte(`{"ok":true,"command":"render","data":{"foo":"bar"}}`)
	out, err := output.ApplyJQ(body, ".", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"foo":"bar"`) {
		t.Errorf("expected JSON object output; got %q", string(out))
	}
}

func TestApplyJQ_NonQuietMode_ScalarString_StaysQuoted(t *testing.T) {
	body := []byte(`{"ok":true,"command":"render","data":{"renderId":"abc123"}}`)
	out, err := output.ApplyJQ(body, ".data.renderId", false)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSuffix(string(out), "\n")
	// Non-quiet mode is "filtering an envelope" — JSON output is
	// appropriate. Strings stay quoted.
	if got != `"abc123"` {
		t.Errorf("got %q, want %q (quotes preserved in non-quiet mode)", got, `"abc123"`)
	}
}
