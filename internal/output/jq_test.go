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
	if strings.TrimSpace(string(out)) != `"abc"` {
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
