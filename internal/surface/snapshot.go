// Package surface generates a deterministic snapshot of the CLI's command and flag surface.
// The snapshot is committed to SURFACE.txt and checked in CI to prevent silent breaking changes.
//
// Exclusion rule (v1.0.4 Class 6 — documented explicitly via Header()):
//   - Cobra builtins (`help` subcommand, `--help` / `--version`) are
//     skipped. They're stable framework-level surfaces we don't own
//     and can't break; tracking them adds noise without a guarantee.
//   - Hidden commands (`Hidden: true`) are skipped. Today that means
//     `urlbox surface` itself — a developer tool exposed for snapshot
//     regeneration only, not part of the user-facing contract.
//   - Hidden flags are skipped similarly.
package surface

import (
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Header returns the documentation comment lines that should prefix
// SURFACE.txt. Each line starts with '#' so readers can filter them
// when scripting. The header is part of the snapshot the surface
// command emits, so SURFACE.txt and `urlbox surface` output stay in
// sync byte-for-byte under `make surface-check`.
func Header() []string {
	return []string{
		"# Urlbox CLI surface contract.",
		"# Every line below is the current public surface: removals and renames",
		"# fail `make surface-check` in CI so they're caught and reviewed, not",
		"# so they're forbidden. From v1.0 this file is the stability promise:",
		"# nothing listed below is removed or renamed within the v1 line without",
		"# a major bump. New entries are always fine.",
		"#",
		"# Excluded by design:",
		"#   - cobra builtins: `help` subcommand, `--help`/`-h`, `--version`",
		"#     (framework-level surfaces we don't own; stable by definition).",
		"#   - hidden commands (currently only `urlbox surface` itself — a",
		"#     developer tool exposed for snapshot regeneration via",
		"#     `make surface-snapshot`).",
		"#   - hidden flags (none today; the mechanism exists for future",
		"#     pre-release flags that don't carry stability guarantees).",
	}
}

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
