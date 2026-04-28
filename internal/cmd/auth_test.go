package cmd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/cmd"
	"github.com/urlbox/urlbox-cli/internal/config"
)

func TestAuth_RequiresAPIKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth"}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on missing --api-key")
	}
}

func TestAuth_WritesConfigFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-key", "sec_xxxxxxxxxxxx"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	if got := config.ResolveAPIKey(); got != "sec_xxxxxxxxxxxx" {
		t.Fatalf("key not persisted; got %q", got)
	}
}

func TestAuth_OutputEnvelopeMasksKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("URLBOX_API_SECRET", "")

	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-key", "sec_supersecretvalue", "--output-format", "json"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d", exit)
	}

	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\nout: %s", err, stdout.String())
	}
	summary, _ := env["summary"].(string)
	if strings.Contains(summary, "supersecretvalue") {
		t.Fatalf("summary should mask the key: %q", summary)
	}
	if !strings.Contains(summary, "sec_") {
		t.Fatalf("summary should show prefix: %q", summary)
	}
	data, _ := env["data"].(map[string]any)
	if mk, _ := data["masked_key"].(string); strings.Contains(mk, "supersecretvalue") {
		t.Fatalf("masked_key leaked secret: %q", mk)
	}
}

func TestAuth_RejectsEmptyKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	exit := cmd.Execute([]string{"auth", "--api-key", ""}, &stdout, &stderr)
	if exit == 0 {
		t.Fatal("expected non-zero exit on empty key")
	}
}

func TestAuth_HasBreadcrumbs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Execute([]string{"auth", "--api-key", "sec_test1234", "--output-format", "json"}, &stdout, &stderr)

	var env map[string]any
	_ = json.Unmarshal(stdout.Bytes(), &env)
	bcs, _ := env["breadcrumbs"].([]any)
	if len(bcs) == 0 {
		t.Fatalf("expected breadcrumbs, got: %v", env["breadcrumbs"])
	}
}
