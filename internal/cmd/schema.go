// internal/cmd/schema.go
package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/schema"
)

// newSchemaCmd builds the parent `urlbox schema` command, which groups
// subcommands that print the JSON Schemas describing Urlbox API payloads.
func newSchemaCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "schema",
		Short: "Inspect embedded JSON Schemas",
		Long:  "Subcommands print the JSON Schemas that describe Urlbox API payloads.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	parent.AddCommand(newSchemaRenderCmd())
	return parent
}

// newSchemaRenderCmd builds the `urlbox schema render` subcommand, which
// prints the embedded render request JSON Schema (Draft 2020-12).
func newSchemaRenderCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "render",
		Short: "Print the JSON Schema for the render request payload",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var data map[string]any
			if err := json.Unmarshal(schema.RenderJSON, &data); err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"Embedded render schema is corrupt: "+err.Error(),
					"This is a build defect. Please open an issue at https://github.com/urlbox/urlbox-cli/issues.",
				)
			}

			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)

			env := output.NewEnvelope(
				"schema render",
				data,
				"Render JSON Schema (Draft 2020-12)",
				[]output.Breadcrumb{
					{Action: "render", Cmd: `urlbox render --json '{"url":"https://example.com"}'`},
				},
			)

			if jqExpr != "" {
				return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			}
			styles := output.NewStylesForWriter(stdout)
			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	}
}
