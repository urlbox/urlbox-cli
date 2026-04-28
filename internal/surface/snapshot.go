// Package surface generates a deterministic snapshot of the CLI's command and flag surface.
// The snapshot is committed to SURFACE.txt and checked in CI to prevent silent breaking changes.
package surface

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Snapshot returns sorted lines describing every visible command and flag in the tree.
// Each command is one line. Each flag is `<command-path> --<flag-name>`. Inherited
// persistent flags appear on every subcommand they reach, so removing a global flag
// causes a loud diff.
func Snapshot(root *cobra.Command) []string {
	lines := map[string]struct{}{}
	walk(root, root.Use, lines)
	out := make([]string, 0, len(lines))
	for l := range lines {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func walk(cmd *cobra.Command, prefix string, out map[string]struct{}) {
	if cmd.Hidden {
		return
	}
	out[prefix] = struct{}{}

	seen := map[string]bool{}
	add := func(f *pflag.Flag) {
		if f.Hidden || seen[f.Name] {
			return
		}
		seen[f.Name] = true
		out[prefix+" --"+f.Name] = struct{}{}
	}
	cmd.PersistentFlags().VisitAll(add)
	cmd.LocalFlags().VisitAll(add)
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		p.PersistentFlags().VisitAll(add)
	}

	for _, sub := range cmd.Commands() {
		if sub.Name() == "help" {
			continue
		}
		walk(sub, prefix+" "+sub.Use, out)
	}
}
