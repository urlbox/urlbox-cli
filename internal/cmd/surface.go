package cmd

import (
	"github.com/spf13/cobra"
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
			w := cmd.OutOrStdout()
			for _, l := range lines {
				if _, err := w.Write([]byte(l + "\n")); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
