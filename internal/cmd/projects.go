package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/urlbox/cli/internal/api"
	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
	"github.com/urlbox/cli/internal/schema"
)

func runProjects(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "projects requires a subcommand")
		return 1
	}

	switch args[0] {
	case "list":
		return runProjectsList(args[1:])
	case "show":
		return runProjectsShow(args[1:])
	case "create":
		return runProjectsCreate(args[1:])
	case "update":
		return runProjectsUpdate(args[1:])
	case "rename":
		return runProjectsRename(args[1:])
	case "enable":
		return runProjectsSetEnabled(args[1:], true)
	case "disable":
		return runProjectsSetEnabled(args[1:], false)
	case "delete":
		return runProjectsDelete(args[1:])
	case "storage":
		return runProjectsStorage(args[1:])
	case "proxy":
		return runProjectsProxy(args[1:])
	case "llm":
		return runProjectsLLM(args[1:])
	case "defaults":
		return runProjectsDefaults(args[1:])
	case "help", "--help", "-h":
		printProjectsUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown projects subcommand %q\n", args[0])
		return 1
	}
}

func runProjectsList(args []string) int {
	fs := flag.NewFlagSet("projects list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, _ := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})

	client := api.New(cfg.APIHost, cfg.APISecret)
	user, err := fetchCurrentUser(client)
	if err != nil {
		return printAPIError("projects list", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	projects, _ := user["projects"].([]interface{})
	envelope := output.Envelope{OK: true, Command: "projects list", Data: projects}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	for _, item := range projects {
		project, _ := item.(map[string]interface{})
		if project == nil {
			continue
		}
		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s\t%s\t%s\t%s\n",
			valueOrEmpty(project["_id"]),
			valueOrEmpty(project["name"]),
			projectEnabledLabel(project),
			valueOrEmpty(project["engineVersion"]),
		)
	}
	return 0
}

func runProjectsShow(args []string) int {
	fs := flag.NewFlagSet("projects show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	var reveal bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full sensitive values")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects show requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	project, err := loadProject(client, positionalArgs[0])
	if err != nil {
		return printAPIError("projects show", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "projects show", Data: project}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	printProjectSummary(project, reveal)
	return 0
}

func runProjectsCreate(args []string) int {
	fs := flag.NewFlagSet("projects create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "project payload")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--json":          true,
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
			return printAPIError("projects create", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
		body = parsed
	}
	if len(positionalArgs) > 0 {
		body["name"] = positionalArgs[0]
	}
	if _, ok := body["name"]; !ok {
		return printAPIError("projects create", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("project name is required"))
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	var response map[string]interface{}
	if err := client.PostJSON("/v1/project", body, &response); err != nil {
		return printAPIError("projects create", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	envelope := output.Envelope{OK: true, Command: "projects create", Data: response}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, "Project created", envelope)
	return 0
}

func runProjectsUpdate(args []string) int {
	fs := flag.NewFlagSet("projects update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "project update payload")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--json":          true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects update requires a project id")
		return 1
	}
	if !jsonInput.set {
		return printAPIError("projects update", output.ResolveFormat(outputFormat, cfg.OutputFormat), fmt.Errorf("--json is required for project updates"))
	}

	body, err := readJSONPayload(jsonInput.value)
	if err != nil {
		return printAPIError("projects update", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	var response map[string]interface{}
	if err := client.PutJSON("/v1/project/"+positionalArgs[0], body, &response); err != nil {
		return printAPIError("projects update", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	envelope := output.Envelope{OK: true, Command: "projects update", Data: response}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, "Project updated", envelope)
	return 0
}

func runProjectsRename(args []string) int {
	fs := flag.NewFlagSet("projects rename", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
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
	if len(positionalArgs) < 2 {
		fmt.Fprintln(os.Stderr, "projects rename requires a project id and new name")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{"name": strings.Join(positionalArgs[1:], " ")},
		"projects rename",
		output.ResolveFormat(outputFormat, cfg.OutputFormat),
		"Project renamed",
	)
}

func runProjectsSetEnabled(args []string, enabled bool) int {
	name := "projects enable"
	if !enabled {
		name = "projects disable"
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintf(os.Stderr, "%s requires a project id\n", name)
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if !enabled && !yes {
		return printAPIError(name, format, fmt.Errorf("disabling requires --yes"))
	}

	message := "Project enabled"
	if !enabled {
		message = "Project disabled"
	}
	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{"enabled": enabled},
		name,
		format,
		message,
	)
}

func runProjectsDelete(args []string) int {
	fs := flag.NewFlagSet("projects delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects delete requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if !yes {
		return printAPIError("projects delete", format, fmt.Errorf("deletion requires --yes"))
	}

	client := api.New(cfg.APIHost, cfg.APISecret)
	var response map[string]interface{}
	if err := client.DeleteJSON("/v1/project/"+positionalArgs[0], &response); err != nil {
		return printAPIError("projects delete", format, err)
	}

	envelope := output.Envelope{OK: true, Command: "projects delete", Data: response}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, "Project deleted", envelope)
	return 0
}

func runProjectsStorage(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "projects storage requires a subcommand")
		return 1
	}
	switch args[0] {
	case "show":
		return runProjectsStorageShow(args[1:])
	case "set":
		return runProjectsStorageSet(args[1:], false)
	case "test":
		return runProjectsStorageSet(args[1:], true)
	case "remove":
		return runProjectsStorageRemove(args[1:])
	case "help", "--help", "-h":
		printProjectsStorageUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown projects storage subcommand %q\n", args[0])
		return 1
	}
}

func runProjectsStorageShow(args []string) int {
	fs := flag.NewFlagSet("projects storage show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	var reveal bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full sensitive values")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects storage show requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	project, err := loadProject(client, positionalArgs[0])
	if err != nil {
		return printAPIError("projects storage show", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	data := map[string]interface{}{
		"s3":    project["s3"],
		"azure": project["azure"],
	}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "projects storage show", Data: data}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	printStorageSummary(project, reveal)
	return 0
}

func runProjectsStorageSet(args []string, testOnly bool) int {
	commandName := "projects storage set"
	if testOnly {
		commandName = "projects storage test"
	}
	fs := flag.NewFlagSet(commandName, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	var provider string
	var key stringValue
	var secret stringValue
	var bucket stringValue
	var region stringValue
	var endpoint stringValue
	var cdnHost stringValue
	var privateBucket boolValue
	var objectLock boolValue
	var azureAccount stringValue
	var azureContainer stringValue
	var azureSASToken stringValue

	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "storage config payload")
	fs.StringVar(&provider, "provider", "", "aws_s3, cloudflare_r2, azure, etc")
	fs.Var(&key, "key", "storage access key")
	fs.Var(&secret, "secret", "storage secret key")
	fs.Var(&bucket, "bucket", "bucket name")
	fs.Var(&region, "region", "bucket region")
	fs.Var(&endpoint, "endpoint", "custom endpoint")
	fs.Var(&cdnHost, "cdn-host", "cdn host")
	fs.Var(&privateBucket, "private-bucket", "private bucket")
	fs.Var(&objectLock, "object-lock", "object lock")
	fs.Var(&azureAccount, "azure-account", "azure account name")
	fs.Var(&azureContainer, "azure-container", "azure container name")
	fs.Var(&azureSASToken, "azure-sas-token", "azure sas token")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")

	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":         true,
		"--api-host":        true,
		"--json":            true,
		"--provider":        true,
		"--key":             true,
		"--secret":          true,
		"--bucket":          true,
		"--region":          true,
		"--endpoint":        true,
		"--cdn-host":        true,
		"--azure-account":   true,
		"--azure-container": true,
		"--azure-sas-token": true,
		"--output-format":   true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintf(os.Stderr, "%s requires a project id\n", commandName)
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)

	payload, endpointPath, err := buildStorageRequest(jsonInput, provider, key, secret, bucket, region, endpoint, cdnHost, privateBucket, objectLock, azureAccount, azureContainer, azureSASToken)
	if err != nil {
		return printAPIError(commandName, output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	var response map[string]interface{}
	if testOnly {
		if err := client.PutJSON("/v1/project/"+positionalArgs[0]+endpointPath, payload, &response); err != nil {
			return printAPIError(commandName, output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
	} else {
		if err := client.PutJSON("/v1/project/"+positionalArgs[0], payload, &response); err != nil {
			return printAPIError(commandName, output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: commandName, Data: response}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	if testOnly {
		_ = output.PrintHuman(format, "Storage connection succeeded", envelope)
	} else {
		_ = output.PrintHuman(format, "Storage configuration updated", envelope)
	}
	return 0
}

func runProjectsStorageRemove(args []string) int {
	fs := flag.NewFlagSet("projects storage remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var outputFormat string
	var profile string
	var apiHost string
	var provider string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.StringVar(&provider, "provider", "", "s3 or azure")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--provider":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects storage remove requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if !yes {
		return printAPIError("projects storage remove", format, fmt.Errorf("removal requires --yes"))
	}

	field := "s3"
	switch provider {
	case "", "s3":
		field = "s3"
	case "azure":
		field = "azure"
	default:
		return printAPIError("projects storage remove", format, fmt.Errorf("unsupported provider %q", provider))
	}

	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{field: nil},
		"projects storage remove",
		format,
		"Storage configuration removed",
	)
}

func runProjectsProxy(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "projects proxy requires a subcommand")
		return 1
	}
	switch args[0] {
	case "show":
		return runProjectsProxyShow(args[1:])
	case "set":
		return runProjectsProxySet(args[1:])
	case "remove":
		return runProjectsProxyRemove(args[1:])
	case "help", "--help", "-h":
		printProjectsProxyUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown projects proxy subcommand %q\n", args[0])
		return 1
	}
}

func runProjectsProxyShow(args []string) int {
	fs := flag.NewFlagSet("projects proxy show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	var reveal bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full sensitive values")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects proxy show requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	project, err := loadProject(client, positionalArgs[0])
	if err != nil {
		return printAPIError("projects proxy show", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	proxies := projectProxyList(project)
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "projects proxy show", Data: proxies}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	if len(proxies) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No proxy configured")
		return 0
	}
	for _, proxy := range proxies {
		_, _ = fmt.Fprintf(os.Stdout, "%s\n", maskSensitive(proxy, reveal))
	}
	return 0
}

func runProjectsProxySet(args []string) int {
	fs := flag.NewFlagSet("projects proxy set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	var url stringValue
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "proxy payload")
	fs.Var(&url, "url", "proxy url")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--json":          true,
		"--url":           true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects proxy set requires a project id")
		return 1
	}

	body := map[string]interface{}{}
	if jsonInput.set {
		parsed, err := readJSONPayload(jsonInput.value)
		if err != nil {
			return printAPIError("projects proxy set", output.ResolveFormat(outputFormat, ""), err)
		}
		body = parsed
	}
	if url.set {
		body["url"] = url.value
	}
	if _, ok := body["url"]; !ok {
		return printAPIError("projects proxy set", output.ResolveFormat(outputFormat, ""), fmt.Errorf("proxy url is required"))
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{
			"proxies": []map[string]interface{}{
				{"url": body["url"]},
			},
		},
		"projects proxy set",
		output.ResolveFormat(outputFormat, cfg.OutputFormat),
		"Proxy configuration updated",
	)
}

func runProjectsProxyRemove(args []string) int {
	fs := flag.NewFlagSet("projects proxy remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects proxy remove requires a project id")
		return 1
	}
	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if !yes {
		return printAPIError("projects proxy remove", format, fmt.Errorf("removal requires --yes"))
	}

	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{"proxies": nil},
		"projects proxy remove",
		format,
		"Proxy configuration removed",
	)
}

func runProjectsLLM(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "projects llm requires a subcommand")
		return 1
	}
	switch args[0] {
	case "show":
		return runProjectsLLMShow(args[1:])
	case "set":
		return runProjectsLLMSet(args[1:], false)
	case "test":
		return runProjectsLLMSet(args[1:], true)
	case "remove":
		return runProjectsLLMRemove(args[1:])
	case "help", "--help", "-h":
		printProjectsLLMUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown projects llm subcommand %q\n", args[0])
		return 1
	}
}

func runProjectsLLMShow(args []string) int {
	fs := flag.NewFlagSet("projects llm show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	var reveal bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full sensitive values")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects llm show requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	project, err := loadProject(client, positionalArgs[0])
	if err != nil {
		return printAPIError("projects llm show", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	llm, _ := project["llm"].(map[string]interface{})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "projects llm show", Data: llm}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	if llm == nil {
		_, _ = fmt.Fprintln(os.Stdout, "No LLM configuration")
		return 0
	}
	keys := sortedKeys(llm)
	for _, key := range keys {
		value := llm[key]
		if key == "key" {
			value = maskSensitive(valueOrEmpty(value), reveal)
		}
		if key == "schema" {
			value = summarizeSchema(value)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s: %v\n", key, value)
	}
	return 0
}

func runProjectsLLMSet(args []string, testOnly bool) int {
	commandName := "projects llm set"
	if testOnly {
		commandName = "projects llm test"
	}
	fs := flag.NewFlagSet(commandName, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	var provider stringValue
	var key stringValue
	var model stringValue
	var prompt stringValue
	var systemPrompt stringValue
	var baseURL stringValue
	var maxTokens intValue
	var temperature stringValue
	var azureResourceName stringValue
	var azureAPIVersion stringValue
	var awsRegion stringValue
	var awsAccessKeyID stringValue
	var awsSecretAccessKey stringValue
	var awsSessionToken stringValue
	var gcpProject stringValue
	var gcpLocation stringValue
	var gcpServiceAccountJSON stringValue
	var schemaInput stringValue

	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "llm payload")
	fs.Var(&provider, "provider", "llm provider")
	fs.Var(&key, "key", "llm api key")
	fs.Var(&model, "model", "llm model")
	fs.Var(&prompt, "prompt", "llm prompt")
	fs.Var(&systemPrompt, "system-prompt", "llm system prompt")
	fs.Var(&baseURL, "base-url", "provider base url")
	fs.Var(&maxTokens, "max-tokens", "max tokens")
	fs.Var(&temperature, "temperature", "temperature")
	fs.Var(&azureResourceName, "azure-resource-name", "azure resource name")
	fs.Var(&azureAPIVersion, "azure-api-version", "azure api version")
	fs.Var(&awsRegion, "aws-region", "aws region")
	fs.Var(&awsAccessKeyID, "aws-access-key-id", "aws access key")
	fs.Var(&awsSecretAccessKey, "aws-secret-access-key", "aws secret key")
	fs.Var(&awsSessionToken, "aws-session-token", "aws session token")
	fs.Var(&gcpProject, "gcp-project", "gcp project")
	fs.Var(&gcpLocation, "gcp-location", "gcp location")
	fs.Var(&gcpServiceAccountJSON, "gcp-service-account-json", "gcp service account json")
	fs.Var(&schemaInput, "schema", "json schema string")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":                  true,
		"--api-host":                 true,
		"--json":                     true,
		"--provider":                 true,
		"--key":                      true,
		"--model":                    true,
		"--prompt":                   true,
		"--system-prompt":            true,
		"--base-url":                 true,
		"--max-tokens":               true,
		"--temperature":              true,
		"--azure-resource-name":      true,
		"--azure-api-version":        true,
		"--aws-region":               true,
		"--aws-access-key-id":        true,
		"--aws-secret-access-key":    true,
		"--aws-session-token":        true,
		"--gcp-project":              true,
		"--gcp-location":             true,
		"--gcp-service-account-json": true,
		"--schema":                   true,
		"--output-format":            true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintf(os.Stderr, "%s requires a project id\n", commandName)
		return 1
	}

	body := map[string]interface{}{}
	if jsonInput.set {
		parsed, err := readJSONPayload(jsonInput.value)
		if err != nil {
			return printAPIError(commandName, output.ResolveFormat(outputFormat, ""), err)
		}
		body = parsed
	}
	mergeFlagString(body, "provider", provider)
	mergeFlagString(body, "key", key)
	mergeFlagString(body, "model", model)
	mergeFlagString(body, "prompt", prompt)
	mergeFlagString(body, "system_prompt", systemPrompt)
	mergeFlagString(body, "base_url", baseURL)
	mergeFlagInt(body, "max_tokens", maxTokens)
	if temperature.set {
		body["temperature"] = toJSONValue(temperature.value)
	}
	mergeFlagString(body, "azure_resource_name", azureResourceName)
	mergeFlagString(body, "azure_api_version", azureAPIVersion)
	mergeFlagString(body, "aws_region", awsRegion)
	mergeFlagString(body, "aws_access_key_id", awsAccessKeyID)
	mergeFlagString(body, "aws_secret_access_key", awsSecretAccessKey)
	mergeFlagString(body, "aws_session_token", awsSessionToken)
	mergeFlagString(body, "gcp_project", gcpProject)
	mergeFlagString(body, "gcp_location", gcpLocation)
	if gcpServiceAccountJSON.set {
		body["gcp_service_account_json"] = gcpServiceAccountJSON.value
	}
	if schemaInput.set {
		body["schema"] = toJSONValue(schemaInput.value)
	}
	if _, ok := body["provider"]; !ok {
		return printAPIError(commandName, output.ResolveFormat(outputFormat, ""), fmt.Errorf("llm provider is required"))
	}
	if _, ok := body["key"]; !ok {
		return printAPIError(commandName, output.ResolveFormat(outputFormat, ""), fmt.Errorf("llm key is required"))
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	response := map[string]interface{}{}
	if testOnly {
		if err := client.PutJSON("/v1/project/"+positionalArgs[0]+"/llm/test", map[string]interface{}{"llm": body}, &response); err != nil {
			return printAPIError(commandName, format, err)
		}
	} else {
		if err := client.PutJSON("/v1/project/"+positionalArgs[0], map[string]interface{}{"llm": body}, &response); err != nil {
			return printAPIError(commandName, format, err)
		}
	}

	envelope := output.Envelope{OK: true, Command: commandName, Data: response}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	if testOnly {
		_ = output.PrintHuman(format, "LLM connection succeeded", envelope)
	} else {
		_ = output.PrintHuman(format, "LLM configuration updated", envelope)
	}
	return 0
}

func runProjectsLLMRemove(args []string) int {
	fs := flag.NewFlagSet("projects llm remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects llm remove requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if !yes {
		return printAPIError("projects llm remove", format, fmt.Errorf("removal requires --yes"))
	}

	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{"llm": nil},
		"projects llm remove",
		format,
		"LLM configuration removed",
	)
}

func runProjectsDefaults(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "projects defaults requires a subcommand")
		return 1
	}
	switch args[0] {
	case "show":
		return runProjectsDefaultsShow(args[1:])
	case "set":
		return runProjectsDefaultsSet(args[1:])
	case "remove":
		return runProjectsDefaultsRemove(args[1:])
	case "help", "--help", "-h":
		printProjectsDefaultsUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown projects defaults subcommand %q\n", args[0])
		return 1
	}
}

func printProjectsUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox projects <subcommand> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Subcommands:")
	_, _ = fmt.Fprintln(os.Stdout, "  list")
	_, _ = fmt.Fprintln(os.Stdout, "  show <project-id> [--reveal]")
	_, _ = fmt.Fprintln(os.Stdout, "  create <name>")
	_, _ = fmt.Fprintln(os.Stdout, "  update <project-id> --json '{...}'")
	_, _ = fmt.Fprintln(os.Stdout, "  rename <project-id> <name>")
	_, _ = fmt.Fprintln(os.Stdout, "  enable <project-id>")
	_, _ = fmt.Fprintln(os.Stdout, "  disable <project-id> --yes")
	_, _ = fmt.Fprintln(os.Stdout, "  delete <project-id> --yes")
	_, _ = fmt.Fprintln(os.Stdout, "  storage <show|set|test|remove> ...")
	_, _ = fmt.Fprintln(os.Stdout, "  proxy <show|set|remove> ...")
	_, _ = fmt.Fprintln(os.Stdout, "  llm <show|set|test|remove> ...")
	_, _ = fmt.Fprintln(os.Stdout, "  defaults <show|set|remove> ...")
}

func printProjectsStorageUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox projects storage <show|set|test|remove> <project-id> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Examples:")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects storage show proj_123")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects storage set proj_123 --json '{\"provider\":\"aws_s3\",\"key\":\"...\",\"secret\":\"...\",\"bucket\":\"renders\",\"region\":\"us-east-1\"}'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects storage test proj_123 --provider azure --azure-account acct --azure-container renders --azure-sas-token 'sv=...'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects storage remove proj_123 --provider azure --yes")
}

func printProjectsProxyUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox projects proxy <show|set|remove> <project-id> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Examples:")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects proxy show proj_123")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects proxy set proj_123 --url 'https://user:pass@proxy.example.com:8080'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects proxy remove proj_123 --yes")
}

func printProjectsLLMUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox projects llm <show|set|test|remove> <project-id> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Examples:")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects llm show proj_123")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects llm set proj_123 --json '{\"provider\":\"anthropic\",\"key\":\"sk-ant-...\"}'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects llm test proj_123 --provider openai --key 'sk-...'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects llm remove proj_123 --yes")
}

func printProjectsDefaultsUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox projects defaults <show|set|remove> <project-id> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Examples:")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects defaults show proj_123")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects defaults set proj_123 --dry-run --json '{\"width\":1920,\"full_page\":true}'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects defaults set proj_123 --merge --json '{\"quality\":90}'")
	_, _ = fmt.Fprintln(os.Stdout, "  urlbox projects defaults remove proj_123 --yes")
}

func runProjectsDefaultsShow(args []string) int {
	fs := flag.NewFlagSet("projects defaults show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profile string
	var apiHost string
	var reveal bool
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full sensitive values")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects defaults show requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)
	project, err := loadProject(client, positionalArgs[0])
	if err != nil {
		return printAPIError("projects defaults show", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	defaults, _ := project["encryptedDefaultOptions"].(map[string]interface{})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	envelope := output.Envelope{OK: true, Command: "projects defaults show", Data: defaults}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}

	if defaults == nil || len(defaults) == 0 {
		_, _ = fmt.Fprintln(os.Stdout, "No default render options")
		return 0
	}
	printDefaultsSummary(defaults, reveal)
	return 0
}

func runProjectsDefaultsSet(args []string) int {
	fs := flag.NewFlagSet("projects defaults set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var jsonInput stringValue
	var outputFormat string
	var profile string
	var apiHost string
	var merge bool
	var dryRun bool
	var formatValue stringValue
	var selector stringValue
	var width intValue
	var height intValue
	var delay intValue
	var timeout intValue
	var quality intValue
	var fullPage boolValue
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.Var(&jsonInput, "json", "default options json")
	fs.BoolVar(&merge, "merge", false, "merge with existing defaults")
	fs.BoolVar(&dryRun, "dry-run", false, "validate only")
	fs.Var(&formatValue, "format", "render format")
	fs.Var(&selector, "selector", "css selector")
	fs.Var(&width, "width", "viewport width")
	fs.Var(&height, "height", "viewport height")
	fs.Var(&delay, "delay", "delay in ms")
	fs.Var(&timeout, "timeout", "timeout in ms")
	fs.Var(&quality, "quality", "image quality")
	fs.Var(&fullPage, "full-page", "capture full page")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--json":          true,
		"--format":        true,
		"--selector":      true,
		"--width":         true,
		"--height":        true,
		"--delay":         true,
		"--timeout":       true,
		"--quality":       true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects defaults set requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	client := api.New(cfg.APIHost, cfg.APISecret)

	defaults := map[string]interface{}{}
	if jsonInput.set {
		parsed, err := readJSONPayload(jsonInput.value)
		if err != nil {
			return printAPIError("projects defaults set", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
		defaults = parsed
	}
	mergeFlagString(defaults, "format", formatValue)
	mergeFlagString(defaults, "selector", selector)
	mergeFlagInt(defaults, "width", width)
	mergeFlagInt(defaults, "height", height)
	mergeFlagInt(defaults, "delay", delay)
	mergeFlagInt(defaults, "timeout", timeout)
	mergeFlagInt(defaults, "quality", quality)
	mergeFlagBool(defaults, "full_page", fullPage)

	manifest, err := schema.Load("render")
	if err != nil {
		return printAPIError("projects defaults set", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}
	properties, _ := manifest["properties"].(map[string]interface{})
	warnings, err := validatePayload(defaults, properties)
	if err != nil {
		return printAPIError("projects defaults set", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
	}

	finalDefaults := defaults
	if merge {
		project, err := loadProject(client, positionalArgs[0])
		if err != nil {
			return printAPIError("projects defaults set", output.ResolveFormat(outputFormat, cfg.OutputFormat), err)
		}
		existing, _ := project["encryptedDefaultOptions"].(map[string]interface{})
		finalDefaults = mergeMaps(existing, defaults)
	}

	result := output.Envelope{
		OK:      true,
		Command: "projects defaults set",
		Data: map[string]interface{}{
			"dry_run":  dryRun,
			"warnings": warnings,
			"defaults": finalDefaults,
		},
	}
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if dryRun {
		if format == "json" {
			_ = output.PrintJSON(result)
			return 0
		}
		_ = output.PrintHuman(format, "Default options validated", result)
		return 0
	}

	var response map[string]interface{}
	if err := client.PutJSON("/v1/project/"+positionalArgs[0], map[string]interface{}{"encryptedDefaultOptions": finalDefaults}, &response); err != nil {
		return printAPIError("projects defaults set", format, err)
	}
	result.Data = response
	if format == "json" {
		_ = output.PrintJSON(result)
		return 0
	}
	_ = output.PrintHuman(format, "Default options updated", result)
	return 0
}

func runProjectsDefaultsRemove(args []string) int {
	fs := flag.NewFlagSet("projects defaults remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	var outputFormat string
	var profile string
	var apiHost string
	fs.StringVar(&profile, "profile", "", "config profile")
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--api-host":      true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "projects defaults remove requires a project id")
		return 1
	}

	cfg := config.Load(config.Options{Profile: profile, APIHost: apiHost, OutputFormat: outputFormat})
	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)
	if !yes {
		return printAPIError("projects defaults remove", format, fmt.Errorf("removal requires --yes"))
	}

	return updateProjectPayload(
		api.New(cfg.APIHost, cfg.APISecret),
		positionalArgs[0],
		map[string]interface{}{"encryptedDefaultOptions": nil},
		"projects defaults remove",
		format,
		"Default options removed",
	)
}

func updateProjectPayload(client *api.Client, projectID string, body map[string]interface{}, command string, format string, message string) int {
	var response map[string]interface{}
	if err := client.PutJSON("/v1/project/"+projectID, body, &response); err != nil {
		return printAPIError(command, format, err)
	}

	envelope := output.Envelope{OK: true, Command: command, Data: response}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_ = output.PrintHuman(format, message, envelope)
	return 0
}

func loadProject(client *api.Client, projectID string) (map[string]interface{}, error) {
	user, err := fetchCurrentUser(client)
	if err != nil {
		return nil, err
	}
	project := findProjectByID(user, projectID)
	if project == nil {
		return nil, fmt.Errorf("project %s not found", projectID)
	}
	return project, nil
}

func buildStorageRequest(
	jsonInput stringValue,
	provider string,
	key stringValue,
	secret stringValue,
	bucket stringValue,
	region stringValue,
	endpoint stringValue,
	cdnHost stringValue,
	privateBucket boolValue,
	objectLock boolValue,
	azureAccount stringValue,
	azureContainer stringValue,
	azureSASToken stringValue,
) (map[string]interface{}, string, error) {
	body := map[string]interface{}{}
	if jsonInput.set {
		parsed, err := readJSONPayload(jsonInput.value)
		if err != nil {
			return nil, "", err
		}
		body = parsed
	}

	targetProvider := provider
	if targetProvider == "" {
		if _, ok := body["azure"]; ok {
			targetProvider = "azure"
		} else if account, ok := body["accountName"]; ok && valueOrEmpty(account) != "" {
			targetProvider = "azure"
		} else if providerValue, ok := body["provider"].(string); ok {
			targetProvider = providerValue
		}
	}

	if targetProvider == "azure" {
		azure := body
		if nested, ok := body["azure"].(map[string]interface{}); ok {
			azure = nested
		}
		mergeFlagString(azure, "accountName", azureAccount)
		mergeFlagString(azure, "containerName", azureContainer)
		mergeFlagString(azure, "sasToken", azureSASToken)
		if _, ok := azure["accountName"]; !ok {
			return nil, "", fmt.Errorf("azure account name is required")
		}
		if _, ok := azure["containerName"]; !ok {
			return nil, "", fmt.Errorf("azure container name is required")
		}
		if _, ok := azure["sasToken"]; !ok {
			return nil, "", fmt.Errorf("azure sas token is required")
		}
		return map[string]interface{}{"azure": azure}, "/azure/test", nil
	}

	mergeFlagString(body, "provider", stringValue{value: provider, set: provider != ""})
	mergeFlagString(body, "key", key)
	mergeFlagString(body, "secret", secret)
	mergeFlagString(body, "bucket", bucket)
	mergeFlagString(body, "region", region)
	mergeFlagString(body, "endpoint", endpoint)
	mergeFlagString(body, "cdn_host", cdnHost)
	mergeFlagBool(body, "private_bucket", privateBucket)
	mergeFlagBool(body, "object_lock", objectLock)
	if _, ok := body["key"]; !ok {
		return nil, "", fmt.Errorf("storage key is required")
	}
	if _, ok := body["secret"]; !ok {
		return nil, "", fmt.Errorf("storage secret is required")
	}
	if _, ok := body["bucket"]; !ok {
		return nil, "", fmt.Errorf("storage bucket is required")
	}
	if _, ok := body["region"]; !ok {
		return nil, "", fmt.Errorf("storage region is required")
	}
	return map[string]interface{}{"s3": body}, "/s3/test", nil
}

func printProjectSummary(project map[string]interface{}, reveal bool) {
	_, _ = fmt.Fprintf(os.Stdout, "ID: %s\n", valueOrEmpty(project["_id"]))
	_, _ = fmt.Fprintf(os.Stdout, "Name: %s\n", valueOrEmpty(project["name"]))
	_, _ = fmt.Fprintf(os.Stdout, "Status: %s\n", projectEnabledLabel(project))
	_, _ = fmt.Fprintf(os.Stdout, "Engine: %s\n", valueOrEmpty(project["engineVersion"]))
	_, _ = fmt.Fprintf(os.Stdout, "API key: %s\n", maskSensitive(valueOrEmpty(project["api_key"]), reveal))
	_, _ = fmt.Fprintf(os.Stdout, "Secret: %s\n", maskSensitive(valueOrEmpty(project["secret"]), reveal))
	if webhookSecret := valueOrEmpty(project["webhookSecret"]); webhookSecret != "" {
		_, _ = fmt.Fprintf(os.Stdout, "Webhook secret: %s\n", maskSensitive(webhookSecret, reveal))
	}
	printStorageSummary(project, reveal)
	if llm, _ := project["llm"].(map[string]interface{}); llm != nil {
		_, _ = fmt.Fprintf(os.Stdout, "LLM: %s\n", llmSummary(llm))
	}
	defaults, _ := project["encryptedDefaultOptions"].(map[string]interface{})
	if len(defaults) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "Defaults: %d option(s)\n", len(defaults))
	}
	proxies := projectProxyList(project)
	if len(proxies) > 0 {
		_, _ = fmt.Fprintf(os.Stdout, "Proxy: %s\n", maskSensitive(proxies[0], reveal))
	}
}

func printStorageSummary(project map[string]interface{}, reveal bool) {
	s3, _ := project["s3"].(map[string]interface{})
	if s3 != nil {
		_, _ = fmt.Fprintf(os.Stdout, "Storage provider: %s\n", valueOrEmpty(s3["provider"]))
		_, _ = fmt.Fprintf(os.Stdout, "Storage bucket: %s\n", valueOrEmpty(s3["bucket"]))
		_, _ = fmt.Fprintf(os.Stdout, "Storage region: %s\n", valueOrEmpty(s3["region"]))
		if endpoint := valueOrEmpty(s3["endpoint"]); endpoint != "" {
			_, _ = fmt.Fprintf(os.Stdout, "Storage endpoint: %s\n", endpoint)
		}
		if key := valueOrEmpty(s3["key"]); key != "" {
			_, _ = fmt.Fprintf(os.Stdout, "Storage key: %s\n", maskSensitive(key, reveal))
		}
		return
	}
	azure, _ := project["azure"].(map[string]interface{})
	if azure != nil {
		_, _ = fmt.Fprintf(os.Stdout, "Azure account: %s\n", valueOrEmpty(azure["accountName"]))
		_, _ = fmt.Fprintf(os.Stdout, "Azure container: %s\n", valueOrEmpty(azure["containerName"]))
		_, _ = fmt.Fprintf(os.Stdout, "Azure SAS token: %s\n", maskSensitive(valueOrEmpty(azure["sasToken"]), reveal))
	}
}

func printDefaultsSummary(defaults map[string]interface{}, reveal bool) {
	keys := sortedKeys(defaults)
	for _, key := range keys {
		value := defaults[key]
		switch key {
		case "headers":
			if !reveal {
				if headers, ok := value.(map[string]interface{}); ok {
					value = fmt.Sprintf("%d header(s) set", len(headers))
				}
			}
		case "cookie", "authorization":
			value = maskSensitive(valueOrEmpty(value), reveal)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s: %v\n", key, value)
	}
}

func projectEnabledLabel(project map[string]interface{}) string {
	if enabled, ok := project["enabled"].(bool); ok && enabled {
		return "enabled"
	}
	if enabled, ok := project["enabled"].(bool); ok && !enabled {
		return "disabled"
	}
	return "unknown"
}

func projectProxyList(project map[string]interface{}) []string {
	proxyItems, _ := project["proxies"].([]interface{})
	proxies := make([]string, 0, len(proxyItems))
	for _, item := range proxyItems {
		proxy, _ := item.(map[string]interface{})
		if proxy == nil {
			continue
		}
		if url := valueOrEmpty(proxy["url"]); url != "" {
			proxies = append(proxies, url)
		}
	}
	return proxies
}

func llmSummary(llm map[string]interface{}) string {
	provider := valueOrEmpty(llm["provider"])
	model := valueOrEmpty(llm["model"])
	if provider == "" {
		return "configured"
	}
	if model == "" {
		return provider
	}
	return provider + " (" + model + ")"
}

func maskSensitive(value string, reveal bool) string {
	if reveal || value == "" {
		return value
	}
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func summarizeSchema(value interface{}) string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return ""
		}
		return "configured JSON schema"
	case map[string]interface{}:
		return fmt.Sprintf("configured JSON schema (%d top-level keys)", len(typed))
	default:
		return "configured"
	}
}

func mergeMaps(base map[string]interface{}, updates map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range updates {
		result[key] = value
	}
	return result
}

func sortedKeys(values map[string]interface{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func toJSONValue(raw string) interface{} {
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}
	return parsed
}
