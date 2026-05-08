package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
)

func TestJQ_FiltersEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"commands", "--output-format", "json", "--jq", ".data.commands[0].name"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, `"`) || !strings.HasSuffix(out, `"`) {
		t.Fatalf("expected quoted command name, got: %q", out)
	}
}

func TestJQ_QuietRunsOnData(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"commands", "--output-format", "quiet", "--jq", ".commands[0].name"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	// Quiet mode strips quotes from scalar strings (matches `jq -r`),
	// so pipelines like `--jq .renderId | xargs ...` work without
	// inheriting the literal `"..."` wrapping the value.
	if out == "" {
		t.Fatalf("expected non-empty result, got: %q", out)
	}
	if strings.HasPrefix(out, `"`) || strings.HasSuffix(out, `"`) {
		t.Fatalf("expected unquoted scalar string in quiet mode, got: %q", out)
	}
}

func TestJQ_InvalidExpr_ExitsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"commands", "--output-format", "json", "--jq", "..["}, &stdout, &stderr)
	if exit != 1 {
		t.Fatalf("expected exit 1 (usage), got %d", exit)
	}
}

func TestJQ_MultipleResults(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"commands", "--output-format", "json", "--jq", ".data.commands[].name"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected multiple lines, got: %q", stdout.String())
	}
}
