package cmd

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
)

func runConfig(args []string) int {
	if len(args) == 0 {
		printConfigUsage()
		return 0
	}

	switch args[0] {
	case "get":
		return runConfigGet(args[1:])
	case "set":
		return runConfigSet(args[1:])
	case "show":
		return runConfigShow(args[1:])
	case "profile":
		return runConfigProfile(args[1:])
	case "help", "--help", "-h":
		printConfigUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n", args[0])
		return 1
	}
}

func runConfigGet(args []string) int {
	fs := flag.NewFlagSet("config get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var profileName string
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.StringVar(&profileName, "profile", "default", "profile name for profile-scoped keys")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--output-format": true,
		"--profile":       true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "config get requires a key")
		return 1
	}

	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config get", output.ResolveFormat(outputFormat, ""), err)
	}

	value, err := readConfigValue(cfg, positionalArgs[0], profileName)
	if err != nil {
		return printAPIError("config get", output.ResolveFormat(outputFormat, ""), err)
	}

	format := output.ResolveFormat(outputFormat, "")
	if format == "json" {
		_ = output.PrintJSON(map[string]interface{}{"key": positionalArgs[0], "profile": profileName, "value": value})
		return 0
	}
	_, _ = fmt.Fprintln(os.Stdout, value)
	return 0
}

func runConfigSet(args []string) int {
	fs := flag.NewFlagSet("config set", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var profileName string
	var outputFormat string
	fs.StringVar(&profileName, "profile", "default", "profile name for profile-scoped keys")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--profile":       true,
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) < 2 {
		fmt.Fprintln(os.Stderr, "config set requires a key and value")
		return 1
	}

	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config set", output.ResolveFormat(outputFormat, ""), err)
	}
	if err := writeConfigValue(&cfg, positionalArgs[0], positionalArgs[1], profileName); err != nil {
		return printAPIError("config set", output.ResolveFormat(outputFormat, ""), err)
	}
	if err := config.Write(cfg); err != nil {
		return printAPIError("config set", output.ResolveFormat(outputFormat, ""), err)
	}

	format := output.ResolveFormat(outputFormat, "")
	envelope := output.Envelope{OK: true, Command: "config set", Data: cfg}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_, _ = fmt.Fprintln(os.Stdout, "Config updated")
	return 0
}

func runConfigShow(args []string) int {
	fs := flag.NewFlagSet("config show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var reveal bool
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full secrets")
	normalizedArgs, _ := normalizeArgs(args, map[string]bool{
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config show", output.ResolveFormat(outputFormat, ""), err)
	}
	format := output.ResolveFormat(outputFormat, "")
	if format == "json" {
		if reveal {
			_ = output.PrintJSON(cfg)
		} else {
			_ = output.PrintJSON(redactConfig(cfg))
		}
		return 0
	}
	_, _ = fmt.Fprintf(os.Stdout, "Config file: %s\n", config.DefaultPath())
	_, _ = fmt.Fprintf(os.Stdout, "Default profile: %s\n", cfg.DefaultProfile)
	names := sortedProfileNames(cfg)
	for _, name := range names {
		prefix := " "
		if name == cfg.DefaultProfile {
			prefix = "*"
		}
		profile := cfg.Profiles[name]
		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s %s\t%s\tkey=%s\tsecret=%s\n",
			prefix,
			name,
			profile.APIHost,
			maskSensitive(profile.APIKey, reveal),
			maskSensitive(profile.APISecret, reveal),
		)
	}
	return 0
}

func runConfigProfile(args []string) int {
	if len(args) == 0 {
		printConfigProfileUsage()
		return 0
	}
	switch args[0] {
	case "create":
		return runConfigProfileCreate(args[1:])
	case "list":
		return runConfigProfileList(args[1:])
	case "show":
		return runConfigProfileShow(args[1:])
	case "delete":
		return runConfigProfileDelete(args[1:])
	case "default", "use":
		return runConfigProfileDefault(args[1:])
	case "help", "--help", "-h":
		printConfigProfileUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown config profile subcommand %q\n", args[0])
		return 1
	}
}

func runConfigProfileCreate(args []string) int {
	fs := flag.NewFlagSet("config profile create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var apiHost string
	var apiKey string
	var apiSecret string
	fs.StringVar(&apiHost, "api-host", "", "api host")
	fs.StringVar(&apiKey, "api-key", "", "api key")
	fs.StringVar(&apiSecret, "api-secret", "", "api secret")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--api-host":   true,
		"--api-key":    true,
		"--api-secret": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "config profile create requires a profile name")
		return 1
	}
	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config profile create", "human", err)
	}
	cfg = config.SetProfile(cfg, positionalArgs[0], config.Profile{
		APIHost:   apiHost,
		APIKey:    apiKey,
		APISecret: apiSecret,
	})
	if err := config.Write(cfg); err != nil {
		return printAPIError("config profile create", "human", err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Profile saved")
	return 0
}

func runConfigProfileList(args []string) int {
	fs := flag.NewFlagSet("config profile list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	normalizedArgs, _ := normalizeArgs(args, map[string]bool{
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config profile list", output.ResolveFormat(outputFormat, ""), err)
	}

	names := sortedProfileNames(cfg)
	format := output.ResolveFormat(outputFormat, "")
	if format == "json" {
		_ = output.PrintJSON(map[string]interface{}{
			"default_profile": cfg.DefaultProfile,
			"profiles":        redactConfig(cfg).Profiles,
		})
		return 0
	}
	for _, name := range names {
		prefix := " "
		if name == cfg.DefaultProfile {
			prefix = "*"
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s %s\t%s\n", prefix, name, cfg.Profiles[name].APIHost)
	}
	return 0
}

func runConfigProfileShow(args []string) int {
	fs := flag.NewFlagSet("config profile show", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var outputFormat string
	var reveal bool
	fs.StringVar(&outputFormat, "output-format", "", "human or json")
	fs.BoolVar(&reveal, "reveal", false, "show full secrets")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{
		"--output-format": true,
	})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "config profile show requires a profile name")
		return 1
	}

	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config profile show", output.ResolveFormat(outputFormat, ""), err)
	}
	profile, err := config.MustProfile(cfg, positionalArgs[0])
	if err != nil {
		return printAPIError("config profile show", output.ResolveFormat(outputFormat, ""), err)
	}
	if !reveal {
		profile.APIKey = maskSensitive(profile.APIKey, false)
		profile.APISecret = maskSensitive(profile.APISecret, false)
	}

	format := output.ResolveFormat(outputFormat, "")
	envelope := output.Envelope{
		OK:      true,
		Command: "config profile show",
		Data: map[string]interface{}{
			"name":    positionalArgs[0],
			"default": positionalArgs[0] == cfg.DefaultProfile,
			"profile": profile,
		},
	}
	if format == "json" {
		_ = output.PrintJSON(envelope)
		return 0
	}
	_, _ = fmt.Fprintf(os.Stdout, "Name: %s\n", positionalArgs[0])
	_, _ = fmt.Fprintf(os.Stdout, "Default: %t\n", positionalArgs[0] == cfg.DefaultProfile)
	_, _ = fmt.Fprintf(os.Stdout, "API host: %s\n", profile.APIHost)
	_, _ = fmt.Fprintf(os.Stdout, "API key: %s\n", profile.APIKey)
	_, _ = fmt.Fprintf(os.Stdout, "API secret: %s\n", profile.APISecret)
	return 0
}

func runConfigProfileDelete(args []string) int {
	fs := flag.NewFlagSet("config profile delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var yes bool
	fs.BoolVar(&yes, "yes", false, "skip confirmation")
	normalizedArgs, positionalArgs := normalizeArgs(args, map[string]bool{})
	if err := fs.Parse(normalizedArgs); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "config profile delete requires a profile name")
		return 1
	}
	if !yes {
		return printAPIError("config profile delete", "human", fmt.Errorf("deletion requires --yes"))
	}

	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config profile delete", "human", err)
	}
	name := positionalArgs[0]
	if name == "default" {
		return printAPIError("config profile delete", "human", fmt.Errorf("cannot delete the built-in default profile"))
	}
	if _, err := config.MustProfile(cfg, name); err != nil {
		return printAPIError("config profile delete", "human", err)
	}
	delete(cfg.Profiles, name)
	if cfg.DefaultProfile == name {
		cfg.DefaultProfile = "default"
	}
	if err := config.Write(cfg); err != nil {
		return printAPIError("config profile delete", "human", err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Profile deleted")
	return 0
}

func runConfigProfileDefault(args []string) int {
	fs := flag.NewFlagSet("config profile default", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	_, positionalArgs := normalizeArgs(args, map[string]bool{})
	if err := fs.Parse([]string{}); err != nil {
		return 1
	}
	if len(positionalArgs) == 0 {
		fmt.Fprintln(os.Stderr, "config profile default requires a profile name")
		return 1
	}
	cfg, err := config.Read()
	if err != nil {
		return printAPIError("config profile default", "human", err)
	}
	if _, err := config.MustProfile(cfg, positionalArgs[0]); err != nil {
		return printAPIError("config profile default", "human", err)
	}
	cfg.DefaultProfile = positionalArgs[0]
	if err := config.Write(cfg); err != nil {
		return printAPIError("config profile default", "human", err)
	}
	_, _ = fmt.Fprintln(os.Stdout, "Default profile updated")
	return 0
}

func printConfigUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox config <subcommand> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Subcommands:")
	_, _ = fmt.Fprintln(os.Stdout, "  get <key> [--profile NAME]")
	_, _ = fmt.Fprintln(os.Stdout, "  set <key> <value> [--profile NAME]")
	_, _ = fmt.Fprintln(os.Stdout, "  show [--reveal]")
	_, _ = fmt.Fprintln(os.Stdout, "  profile <create|list|show|delete|default|use> ...")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Config keys:")
	_, _ = fmt.Fprintln(os.Stdout, "  default_profile")
	_, _ = fmt.Fprintln(os.Stdout, "  api_host")
	_, _ = fmt.Fprintln(os.Stdout, "  api_key")
	_, _ = fmt.Fprintln(os.Stdout, "  api_secret")
}

func printConfigProfileUsage() {
	_, _ = fmt.Fprintln(os.Stdout, "urlbox config profile <subcommand> [options]")
	_, _ = fmt.Fprintln(os.Stdout, "")
	_, _ = fmt.Fprintln(os.Stdout, "Subcommands:")
	_, _ = fmt.Fprintln(os.Stdout, "  create <name> --api-host URL [--api-key KEY] [--api-secret SECRET]")
	_, _ = fmt.Fprintln(os.Stdout, "  list")
	_, _ = fmt.Fprintln(os.Stdout, "  show <name> [--reveal]")
	_, _ = fmt.Fprintln(os.Stdout, "  delete <name> --yes")
	_, _ = fmt.Fprintln(os.Stdout, "  default <name>")
	_, _ = fmt.Fprintln(os.Stdout, "  use <name>")
}

func readConfigValue(cfg config.File, key string, profileName string) (interface{}, error) {
	switch key {
	case "default_profile":
		return cfg.DefaultProfile, nil
	case "api_host":
		profile, err := config.MustProfile(cfg, profileName)
		if err != nil {
			return nil, err
		}
		return profile.APIHost, nil
	case "api_key":
		profile, err := config.MustProfile(cfg, profileName)
		if err != nil {
			return nil, err
		}
		return profile.APIKey, nil
	case "api_secret":
		profile, err := config.MustProfile(cfg, profileName)
		if err != nil {
			return nil, err
		}
		return profile.APISecret, nil
	default:
		return nil, fmt.Errorf("unknown config key %q", key)
	}
}

func writeConfigValue(cfg *config.File, key string, value string, profileName string) error {
	switch key {
	case "default_profile":
		if _, err := config.MustProfile(*cfg, value); err != nil {
			return err
		}
		cfg.DefaultProfile = value
		return nil
	case "api_host", "api_key", "api_secret":
		profile, ok := config.GetProfile(*cfg, profileName)
		if !ok {
			profile = config.Profile{}
		}
		switch key {
		case "api_host":
			profile.APIHost = value
		case "api_key":
			profile.APIKey = value
		case "api_secret":
			profile.APISecret = value
		}
		*cfg = config.SetProfile(*cfg, profileName, profile)
		return nil
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
}

func sortedProfileNames(cfg config.File) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func redactConfig(cfg config.File) config.File {
	redacted := cfg
	redacted.Profiles = map[string]config.Profile{}
	for name, profile := range cfg.Profiles {
		redacted.Profiles[name] = config.Profile{
			APIHost:   profile.APIHost,
			APIKey:    maskSensitive(profile.APIKey, false),
			APISecret: maskSensitive(profile.APISecret, false),
		}
	}
	return redacted
}
