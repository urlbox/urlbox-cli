package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/urlbox/urlbox-cli/internal/output"
)

func newUsageCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "usage",
		Short: "Show the organisation's render usage for the current period",
		Args:  cobra.NoArgs,
		RunE:  runUsage,
	}
	attachSessionRetryFlags(c)
	return c
}

func runUsage(cmd *cobra.Command, _ []string) error {
	sess, cliErr := loadSession(cmd)
	if cliErr != nil {
		return cliErr
	}
	var usage struct {
		RendersUsed int `json:"rendersUsed"`
		RenderQuota int `json:"renderQuota"`
		Period      struct {
			Start string `json:"start"`
			End   string `json:"end"`
		} `json:"period"`
	}
	if err := sess.Client.GetJSON(context.Background(), "/v2/usage", &usage); err != nil {
		return asCLIError(err)
	}
	data := map[string]any{
		"renders_used":         usage.RendersUsed,
		"render_quota":         usage.RenderQuota,
		"current_period_start": usage.Period.Start,
		"current_period_end":   usage.Period.End,
	}
	summary := fmt.Sprintf("Renders used: %d / %d", usage.RendersUsed, usage.RenderQuota)
	env := output.NewEnvelope("usage", data, summary, nil)
	env.SetKV([][2]string{
		{"Renders used", fmt.Sprintf("%d", usage.RendersUsed)},
		{"Render quota", fmt.Sprintf("%d", usage.RenderQuota)},
		{"Period start", usage.Period.Start},
		{"Period end", usage.Period.End},
	})
	return writeEnvelopeWithQuietData(cmd, env, fmt.Sprintf("%d", usage.RendersUsed))
}
