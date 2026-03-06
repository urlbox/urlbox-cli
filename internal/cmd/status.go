package cmd

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
)

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var wait bool
	var timeoutSeconds int
	var outputFormat string
	var profile string
	var apiHost string

	fs.BoolVar(&wait, "wait", false, "wait for the render to complete")
	fs.IntVar(&timeoutSeconds, "timeout", 60, "wait timeout in seconds")
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "status requires a render id")
		return 1
	}

	cfg := config.Load(config.Options{
		Profile:      profile,
		APIHost:      apiHost,
		OutputFormat: outputFormat,
	})

	client := api.New(cfg.APIHost, cfg.APISecret)
	renderID := fs.Arg(0)
	deadline := time.Now().Add(time.Duration(timeoutSeconds) * time.Second)
	var response map[string]interface{}

	for {
		err := client.GetJSON("/v1/render/"+renderID, &response)
		if err != nil {
			return printAPIError("status", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}

		status := ""
		if value, ok := response["status"].(string); ok {
			status = value
		}

		if !wait || status == "succeeded" || status == "failed" || status == "not-found" {
			break
		}

		if time.Now().After(deadline) {
			return printAPIError("status", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("timed out waiting for render %s", renderID))
		}

		time.Sleep(2 * time.Second)
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{
		OK:      true,
		Command: "status",
		Data:    response,
	}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	message := fmt.Sprintf("Render %s: %v", renderID, response["status"])
	_ = output.PrintHuman(format, message, envelope)
	return 0
}
