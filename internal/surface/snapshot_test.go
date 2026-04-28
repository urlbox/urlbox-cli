package surface_test

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/surface"
)

func newFixture() *cobra.Command {
	root := &cobra.Command{Use: "urlbox"}
	root.PersistentFlags().String("output-format", "", "")
	root.PersistentFlags().String("hidden-flag", "", "")
	_ = root.PersistentFlags().MarkHidden("hidden-flag")

	cmds := &cobra.Command{Use: "commands"}
	cmds.Flags().Bool("verbose", false, "")
	root.AddCommand(cmds)

	upgrade := &cobra.Command{Use: "upgrade"}
	upgrade.Flags().Bool("check", false, "")
	root.AddCommand(upgrade)

	hidden := &cobra.Command{Use: "secret", Hidden: true}
	root.AddCommand(hidden)

	return root
}

func TestSnapshot_IncludesAllVisibleCommands(t *testing.T) {
	got := surface.Snapshot(newFixture())
	want := []string{
		"urlbox",
		"urlbox --output-format",
		"urlbox commands",
		"urlbox commands --output-format",
		"urlbox commands --verbose",
		"urlbox upgrade",
		"urlbox upgrade --check",
		"urlbox upgrade --output-format",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

func TestSnapshot_SkipsHidden(t *testing.T) {
	got := surface.Snapshot(newFixture())
	for _, line := range got {
		if line == "urlbox secret" || line == "urlbox --hidden-flag" {
			t.Fatalf("hidden entry leaked: %q", line)
		}
	}
}

func TestSnapshot_Sorted(t *testing.T) {
	got := surface.Snapshot(newFixture())
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Fatalf("not sorted: %q before %q", got[i-1], got[i])
		}
	}
}
