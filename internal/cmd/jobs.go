package cmd

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/jobs"
	"github.com/urlbox/cli/internal/output"
)

func runJobs(args []string) int {
	fs := flag.NewFlagSet("jobs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var wait bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.BoolVar(&wait, "wait", false, "wait for active jobs to complete")
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)

	file, err := jobs.Read()
	if err != nil {
		return printAPIError("jobs", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	selected := file.Jobs
	if len(positionalArgs) > 0 {
		id, err := strconv.Atoi(positionalArgs[0])
		if err != nil {
			return printAPIError("jobs", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
		job, ok, err := jobs.Find(id)
		if err != nil {
			return printAPIError("jobs", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
		if !ok {
			return printAPIError("jobs", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("job %d not found", id))
		}
		selected = []jobs.Job{job}
	}

	if wait {
		for index := range selected {
			for {
				statusBody, err := fetchStatusBody(client, selected[index].RenderID)
				if err != nil {
					return printAPIError("jobs", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
				}
				if status, _ := statusBody["status"].(string); status != "created" && status != "retrying" {
					selected[index].Status = status
					selected[index].Result = statusBody
					_ = jobs.Update(selected[index])
					break
				}
				time.Sleep(2 * time.Second)
			}
		}
	} else {
		for index := range selected {
			statusBody, err := fetchStatusBody(client, selected[index].RenderID)
			if err == nil {
				selected[index].Status, _ = statusBody["status"].(string)
				selected[index].Result = statusBody
				_ = jobs.Update(selected[index])
			}
		}
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "jobs", Data: selected}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, fmt.Sprintf("%d jobs", len(selected)), envelope)
	return 0
}

func fetchStatusBody(client *api.Client, renderID string) (map[string]interface{}, error) {
	var response map[string]interface{}
	if err := client.GetJSON("/v1/render/"+renderID, &response); err != nil {
		return nil, err
	}
	return response, nil
}
