package cmd

import (
	"fmt"
	"os"

	"github.com/urlbox/cli/internal/version"
)

func Execute(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 0
	}

	switch args[0] {
	case "schema":
		return runSchema(args[1:])
	case "render":
		return runRender(args[1:])
	case "batch":
		return runBatch(args[1:])
	case "jobs":
		return runJobs(args[1:])
	case "projects":
		return runProjects(args[1:])
	case "config":
		return runConfig(args[1:])
	case "status":
		return runStatus(args[1:])
	case "usage":
		return runUsage(args[1:])
	case "webhook":
		if !internalFeatures {
			fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
			printUsage()
			return 1
		}
		return runWebhook(args[1:])
	case "auth":
		return runAuth(args[1:])
	case "version":
		fmt.Fprintf(os.Stdout, "urlbox %s\n", version.Version)
		return 0
	case "help", "--help", "-h":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		printUsage()
		return 1
	}
}

func printUsage() {
	fmt.Fprintln(os.Stdout, "urlbox <command> [options]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Commands:")
	fmt.Fprintln(os.Stdout, "  auth      Authentication helpers")
	fmt.Fprintln(os.Stdout, "  batch     Run multiple renders")
	fmt.Fprintln(os.Stdout, "  config    Manage local CLI configuration")
	fmt.Fprintln(os.Stdout, "  jobs      Manage background renders")
	fmt.Fprintln(os.Stdout, "  projects  Manage projects")
	fmt.Fprintln(os.Stdout, "  schema    Inspect CLI option manifests")
	fmt.Fprintln(os.Stdout, "  render    Assemble and validate a render payload")
	fmt.Fprintln(os.Stdout, "  status    Check async render status")
	fmt.Fprintln(os.Stdout, "  usage     View usage summary")
	fmt.Fprintln(os.Stdout, "  version   Print CLI version")
	if internalFeatures {
		fmt.Fprintln(os.Stdout, "  webhook   Manage webhooks (internal)")
	}
}
