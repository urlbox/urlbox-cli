// internal/output/help_test.go
package output_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func TestBuildAgentHelp_RootHasNameAndUsage(t *testing.T) {
	root := &cobra.Command{Use: "urlbox", Short: "Urlbox CLI"}
	root.PersistentFlags().String("output-format", "", "Output format")

	env := output.BuildAgentHelp(root)

	if !env.OK {
		t.Fatal("ok must be true")
	}
	if env.Command != "urlbox" {
		t.Fatalf("command=%s", env.Command)
	}
	data, _ := json.Marshal(env.Data)
	if !strings.Contains(string(data), `"name":"urlbox"`) {
		t.Fatalf("name missing: %s", data)
	}
	if !strings.Contains(string(data), `"usage"`) {
		t.Fatalf("usage missing: %s", data)
	}
}

func TestBuildAgentHelp_FlagsHaveTypes(t *testing.T) {
	c := &cobra.Command{Use: "render"}
	c.Flags().String("format", "", "format")
	c.Flags().Int("width", 0, "width")
	c.Flags().Bool("full-page", false, "full page")

	env := output.BuildAgentHelp(c)
	data, _ := json.Marshal(env.Data)
	for _, want := range []string{`"format"`, `"width"`, `"full-page"`, `"string"`, `"int"`, `"bool"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s in: %s", want, data)
		}
	}
}

func TestBuildAgentHelp_IncludesSubcommands(t *testing.T) {
	root := &cobra.Command{Use: "urlbox"}
	root.AddCommand(&cobra.Command{Use: "commands", Short: "List commands"})
	root.AddCommand(&cobra.Command{Use: "doctor", Short: "Diagnose"})
	root.AddCommand(&cobra.Command{Use: "secret", Hidden: true})

	env := output.BuildAgentHelp(root)
	data, _ := json.Marshal(env.Data)
	if !strings.Contains(string(data), `"commands"`) || !strings.Contains(string(data), `"doctor"`) {
		t.Fatalf("subcommands missing: %s", data)
	}
	if strings.Contains(string(data), `"secret"`) {
		t.Fatalf("hidden subcommand leaked: %s", data)
	}
}

func TestBuildAgentHelp_HasBreadcrumbs(t *testing.T) {
	c := &cobra.Command{Use: "urlbox"}
	env := output.BuildAgentHelp(c)
	if len(env.Breadcrumbs) == 0 {
		t.Fatal("expected at least one breadcrumb")
	}
}
