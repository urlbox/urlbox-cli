package cmd

import (
	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/skills"
)

func newSkillCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "skill",
		Short: "Agent skill content",
		Long:  "Subcommands for working with the embedded SKILL.md.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	parent.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print the embedded SKILL.md",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)

			if format == output.FormatText && jqExpr == "" {
				_, err := stdout.Write([]byte(skills.Content))
				return err
			}

			env := output.NewEnvelope(
				"skill show",
				map[string]string{"skill": skills.Content},
				"Embedded SKILL.md",
				nil,
			)
			if jqExpr != "" {
				return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			}
			styles := output.NewStylesForWriter(stdout)
			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	})
	return parent
}
