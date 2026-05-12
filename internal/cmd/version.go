// internal/cmd/version.go — Round 8 II: cobra's --version flag emits a
// plain-text line via VersionTemplate, which doesn't honor
// --output-format. For agents that want structured version info, this
// subcommand emits the same envelope every other command does.
//
// --version (the flag) remains as-is — plain-text version is the
// universal CLI convention and we don't want to break scripts that
// parse it. Users who want JSON use `urlbox version --output-format json`.
package cmd

import (
	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version, commit, and build date",
		Long: `Print the CLI version. Honors --output-format:

  json  -> envelope with { version, commit, date } in data
  text  -> single-line summary
  quiet -> just the version string

The --version flag at the root prints plain text only and is intended
for scripts that grep "vX.Y.Z" out of "urlbox --version".`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := output.NewEnvelope(
				"version",
				map[string]string{
					"version": version.Version,
					"commit":  version.Commit,
					"date":    version.Date,
				},
				"urlbox "+version.Version+" (commit: "+version.Commit+", built: "+version.Date+")",
				nil,
			)
			return writeEnvelopeWithQuietData(cmd, env, version.Version)
		},
	}
}
