package cmd

import (
	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/surface"
)

func newSurfaceCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:    "surface",
		Short:  "Print the CLI surface snapshot (developer tool)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			lines := surface.Snapshot(root)
			// Round 8 MM: only emit JSON envelope when the user
			// EXPLICITLY passes --output-format json. The Makefile's
			// `surface > SURFACE.txt` redirects stdout (not a TTY) and
			// would otherwise auto-resolve to json — wrecking the
			// surface snapshot consumed by surface-check.
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			if formatFlag == string(output.FormatJSON) {
				env := output.NewEnvelope(
					"surface",
					map[string]any{"lines": lines},
					"",
					nil,
				)
				return writeEnvelope(cmd, env)
			}
			w := cmd.OutOrStdout()
			// v1.0.4 Class 6: emit the exclusion-rule header so SURFACE.txt
			// is self-documenting. Header lines are '#'-prefixed comments
			// readers can filter when scripting.
			for _, h := range surface.Header() {
				if _, err := w.Write([]byte(h + "\n")); err != nil {
					return err
				}
			}
			for _, l := range lines {
				if _, err := w.Write([]byte(l + "\n")); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
