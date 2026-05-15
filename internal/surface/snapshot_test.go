package surface_test

import (
	"reflect"
	"strings"
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

// ─── v1.0.4 Class 6 — explicit exclusion-rule header ────────────────
//
// Invariant: the surface contract documents what it covers AND what it
// deliberately excludes, so a reader of SURFACE.txt knows the bounds
// of the stability promise. Pre-1.0.4 the snapshot quietly skipped
// hidden commands and the cobra `help` builtin, leaving readers
// unsure whether "missing from SURFACE.txt" meant "broken" or
// "intentional".

func TestHeader_DocumentsExclusionRule(t *testing.T) {
	header := surface.Header()
	if len(header) == 0 {
		t.Fatal("Header must not be empty")
	}
	// Every header line must start with '#' so it's a clear comment in
	// SURFACE.txt and easy to filter when scripting.
	for i, line := range header {
		if !strings.HasPrefix(line, "#") {
			t.Errorf("header[%d] = %q, want '#' prefix", i, line)
		}
	}
	joined := strings.Join(header, "\n")
	required := []string{
		"cobra builtins",
		"--help",
		"--version",
		"hidden commands",
		"surface",
		"make surface-check",
	}
	for _, s := range required {
		if !strings.Contains(joined, s) {
			t.Errorf("header missing %q (header must document the exclusion rule clearly)", s)
		}
	}
}
