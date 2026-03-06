//go:build internal

package cmd

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
)

func runWebhook(args []string) int {
	if len(args) == 0 {
		printWebhookUsage()
		return 0
	}

	switch args[0] {
	case "list":
		return runWebhookList(args[1:])
	case "show":
		return runWebhookShow(args[1:])
	case "create", "set":
		return runWebhookCreate(args[1:])
	case "delete", "remove":
		return runWebhookDelete(args[1:])
	case "help", "--help", "-h":
		printWebhookUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown webhook subcommand %q\n", args[0])
		return 1
	}
}

func runWebhookList(args []string) int {
	fs := flag.NewFlagSet("webhook list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var projectID string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&projectID, "project", "", "project id")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, _ := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--project":       true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})

	client := api.New(cfg.APIHost, cfg.APISecret)
	user, err := fetchCurrentUser(client)
	if err != nil {
		return printAPIError("webhook list", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}
	if projectID == "" {
		projectID = inferCurrentProjectID(client)
	}
	project := findProjectByID(user, projectID)
	if project == nil {
		return printAPIError("webhook list", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("project %s not found", projectID))
	}

	webhooks, _ := project["webhooks"].([]interface{})
	envelope := output.Envelope{OK: true, Command: "webhook list", Data: webhooks}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	if len(webhooks) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No webhooks configured")
		return 0
	}
	for _, item := range webhooks {
		webhook, _ := item.(map[string]interface{})
		if webhook == nil {
			continue
		}
		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s\t%s\tenabled=%v\tevents=%s\n",
			valueOrEmpty(webhook["id"]),
			valueOrEmpty(webhook["url"]),
			webhook["enabled"],
			strings.Join(interfaceSliceToStrings(webhook["events"]), ","),
		)
	}
	return 0
}

func runWebhookShow(args []string) int {
	fs := flag.NewFlagSet("webhook show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var projectID string
	var profile string
	var apiHost string
	var reveal bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&projectID, "project", "", "project id")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full secrets")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--project":       true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "webhook show requires a webhook id")
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	webhook, err := loadWebhook(client, projectID, positionalArgs[0])
	if err != nil {
		return printAPIError("webhook show", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "webhook show", Data: webhook}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	keys := sortedMapKeys(webhook)
	for _, key := range keys {
		value := webhook[key]
		if key == "key" {
			value = maskSensitive(valueOrEmpty(value), reveal)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s: %v\n", key, value)
	}
	return 0
}

func runWebhookCreate(args []string) int {
	fs := flag.NewFlagSet("webhook create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	var src string
	var events csvValue
	var firesOn csvValue
	var enabled boolValue
	var dryRun bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "webhook payload")
	fs.StringVar(&src, "src", "", "webhook source label")
	fs.Var(&events, "events", "comma-separated events")
	fs.Var(&firesOn, "fires-on", "comma-separated methods: GET,POST,ALL")
	fs.Var(&enabled, "enabled", "enabled state")
	fs.BoolVar(&dryRun, "dry-run", false, "validate without creating")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--json":          true,
		"--src":           true,
		"--events":        true,
		"--fires-on":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	body := map[string]interface{}{}
	if jsonInput.set {
		parsed, err := readJSONPayload(jsonInput.value)
		if err != nil {
			return printAPIError("webhook create", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
		body = parsed
	}
	if len(positionalArgs) > 0 {
		body["url"] = positionalArgs[0]
	}
	if src != "" {
		body["src"] = src
	}
	if events.set {
		body["events"] = events.values
	}
	if firesOn.set {
		body["fires_on"] = firesOn.values
	}
	if enabled.set {
		body["enabled"] = enabled.value
	}
	if err := validateWebhookPayload(body); err != nil {
		return printAPIError("webhook create", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	envelope := output.Envelope{OK: true, Command: "webhook create", Data: body}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if dryRun {
		if format == "json" {
			_ = output.PrintJSON(envelope)
			return 0
		}
		_ = output.PrintHuman(format, "Webhook payload validated", envelope)
		return 0
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	var response map[string]interface{}
	if err := client.PostJSON("/v1/webhook", body, &response); err != nil {
		return printAPIError("webhook create", format, err)
	}

	envelope.Data = response
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, "Webhook created", envelope)
	return 0
}

func runWebhookDelete(args []string) int {
	fs := flag.NewFlagSet("webhook delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var dryRun bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.BoolVar(&dryRun, "dry-run", false, "validate only")
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
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "webhook delete requires a webhook id")
		return 1
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{
		OK:      true,
		Command: "webhook delete",
		Data: map[string]interface{}{
			"webhook_id": positionalArgs[0],
			"dry_run":    dryRun,
		},
	}
	if dryRun {
		if format == "json" {
			_ = output.PrintJSON(envelope)
			return 0
		}
		_ = output.PrintHuman(format, "Webhook deletion validated", envelope)
		return 0
	}
	if !yes {
		return printAPIError("webhook delete", format, fmt.Errorf("deletion requires --yes"))
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	var response map[string]interface{}
	if err := client.DeleteJSON("/v1/webhook/"+positionalArgs[0], &response); err != nil {
		return printAPIError("webhook delete", format, err)
	}

	envelope.Data = response
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, "Webhook deleted", envelope)
	return 0
}

func printWebhookUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox webhook <subcommand> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Subcommands:")
	_, _ = fmt.Fprintln(os.Stdout, "  list [--project PROJECT_ID]")
	_, _ = fmt.Fprintln(os.Stdout, "  show <webhook-id> [--project PROJECT_ID]")
	_, _ = fmt.Fprintln(os.Stdout, "  create <url> [--events a,b] [--fires-on POST] [--src zapier] [--dry-run]")
	_, _ = fmt.Fprintln(os.Stdout, "  set <url> ...        alias for create")
	_, _ = fmt.Fprintln(os.Stdout, "  delete <webhook-id> --yes [--dry-run]")
	_, _ = fmt.Fprintln(os.Stdout, "  remove <webhook-id> alias for delete")
}

func inferCurrentProjectID(client *api.Client) string {
	var response map[string]interface{}
	if err := client.GetJSON("/v1/users/me", &response); err != nil {
		return ""
	}
	user, _ := response["user"].(map[string]interface{})
	project, _ := user["project"].(map[string]interface{})
	id := valueOrEmpty(project["_id"])
	if id == "" {
		id = valueOrEmpty(project["id"])
	}
	return id
}

func loadWebhook(client *api.Client, projectID string, webhookID string) (map[string]interface{}, error) {
	user, err := fetchCurrentUser(client)
	if err != nil {
		return nil, err
	}
	if projectID == "" {
		projectID = inferCurrentProjectID(client)
	}
	project := findProjectByID(user, projectID)
	if project == nil {
		return nil, fmt.Errorf("project %s not found", projectID)
	}
	webhooks, _ := project["webhooks"].([]interface{})
	for _, item := range webhooks {
		webhook, _ := item.(map[string]interface{})
		if webhook == nil {
			continue
		}
		if valueOrEmpty(webhook["id"]) == webhookID || valueOrEmpty(webhook["_id"]) == webhookID {
			return webhook, nil
		}
	}
	return nil, fmt.Errorf("webhook %s not found", webhookID)
}

func validateWebhookPayload(body map[string]interface{}) error {
	rawURL, ok := body["url"]
	if !ok || valueOrEmpty(rawURL) == "" {
		return fmt.Errorf("webhook url is required")
	}
	parsed, err := url.Parse(valueOrEmpty(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid webhook url")
	}
	if rawEvents, ok := body["events"]; ok {
		if _, ok := rawEvents.([]string); !ok {
			if _, ok := rawEvents.([]interface{}); !ok {
				return fmt.Errorf("events must be an array")
			}
		}
	}
	if rawFiresOn, ok := body["fires_on"]; ok {
		values := interfaceSliceToStrings(rawFiresOn)
		if len(values) == 0 {
			return fmt.Errorf("fires_on must be an array")
		}
		for _, value := range values {
			switch strings.ToUpper(value) {
			case "GET", "POST", "ALL":
			default:
				return fmt.Errorf("fires_on values must be GET, POST, or ALL")
			}
		}
	}
	return nil
}

type csvValue struct {
	values []string
	set    bool
}

func (v *csvValue) String() string { return strings.Join(v.values, ",") }
func (v *csvValue) Set(value string) error {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	v.values = values
	v.set = true
	return nil
}

func interfaceSliceToStrings(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		sort.Strings(result)
		return result
	default:
		return nil
	}
}

func sortedMapKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
