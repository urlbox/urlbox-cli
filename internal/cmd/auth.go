package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
)

func runAuth(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "auth requires a subcommand")
		return 1
	}

	switch args[0] {
	case "whoami":
		return runAuthWhoami(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown auth subcommand %q\n", args[0])
		return 1
	}
}

func runAuthWhoami(args []string) int {
	fs := flag.NewFlagSet("auth whoami", flag.ContinueOnError)
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

	cfg := config.Load(config.Options{
		Profile:      profile,
		APIHost:      apiHost,
		OutputFormat: outputFormat,
	})

	client := api.New(cfg.APIHost, cfg.APISecret)
	var response map[string]interface{}
	if err := client.GetJSON("/v1/users/me", &response); err != nil {
		return printAPIError("auth whoami", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{
		OK:      true,
		Command: "auth whoami",
		Data:    response,
	}

	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	user, _ := response["user"].(map[string]interface{})
	if user == nil {
		_ = output.PrintHuman(format, "Authenticated", envelope)
		return 0
	}

	projectCount := 0
	if projects, ok := user["projects"].([]interface{}); ok {
		projectCount = len(projects)
	}
	message := fmt.Sprintf(
		"Authenticated as %v (%d projects)",
		user["email"],
		projectCount,
	)
	_ = output.PrintHuman(format, message, envelope)
	return 0
}
