package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
)

func runUsage(args []string) int {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})

	client := api.New(cfg.APIHost, cfg.APISecret)
	user, err := fetchCurrentUser(client)
	if err != nil {
		return printAPIError("usage", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	summary := map[string]interface{}{
		"renders_used":         user["rendersUsed"],
		"current_period_start": user["currentPeriodStart"],
		"current_period_end":   user["currentPeriodEnd"],
		"plan":                 user["plan"],
		"stats":                user["stats"],
	}

	envelope := output.Envelope{OK: true, Command: "usage", Data: summary}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, fmt.Sprintf("Renders used: %v", user["rendersUsed"]), envelope)
	return 0
}
