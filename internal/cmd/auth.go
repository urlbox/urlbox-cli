package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func newAuthCmd() *cobra.Command {
	var apiKey string
	c := &cobra.Command{
		Use:   "auth",
		Short: "Configure API credentials",
		Long: `Save an Urlbox API key to the local config file.
The env var URLBOX_API_SECRET takes precedence at runtime.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apiKey == "" {
				return output.NewCLIError(
					output.ErrUsage,
					"missing --api-key",
					"Pass --api-key <key> or export URLBOX_API_SECRET",
				)
			}
			if err := config.Save(&config.Config{APIKey: apiKey}); err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"failed to save config",
					err.Error(),
				)
			}

			masked := maskKey(apiKey)
			env := output.NewEnvelope(
				"auth",
				map[string]string{
					"masked_key":  masked,
					"config_path": config.Path(),
				},
				fmt.Sprintf("API key configured (%s)", masked),
				[]output.Breadcrumb{
					{Action: "verify", Cmd: "urlbox doctor"},
					{Action: "render", Cmd: "urlbox render <url>"},
				},
			)

			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)
			styles := output.NewStylesForWriter(stdout)

			if jqExpr != "" {
				return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			}
			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	}
	c.Flags().StringVar(&apiKey, "api-key", "", "Urlbox API key (required)")
	return c
}

// maskKey returns a redacted form of the key for safe display.
func maskKey(s string) string {
	if len(s) < 8 {
		return "***"
	}
	return s[:4] + "…" + s[len(s)-2:]
}
