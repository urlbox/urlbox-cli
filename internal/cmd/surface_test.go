package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestSurface_Command_OutputsSnapshot(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"surface"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "urlbox\n") {
		t.Fatalf("missing root entry; got:\n%s", out)
	}
	if !strings.Contains(out, "urlbox commands") {
		t.Fatalf("missing commands entry; got:\n%s", out)
	}
	if !strings.Contains(out, "urlbox upgrade") {
		t.Fatalf("missing upgrade entry; got:\n%s", out)
	}
	if !strings.Contains(out, "--output-format") {
		t.Fatalf("missing --output-format; got:\n%s", out)
	}
}

func TestSurface_Command_HiddenFromCatalog(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"commands", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if strings.Contains(stdout.String(), `"name":"surface"`) ||
		strings.Contains(stdout.String(), `"name": "surface"`) {
		t.Fatal("surface command should not appear in commands catalog")
	}
}
