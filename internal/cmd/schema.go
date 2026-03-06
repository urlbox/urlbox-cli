package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/urlbox/cli/internal/config"
	"github.com/urlbox/cli/internal/output"
	"github.com/urlbox/cli/internal/schema"
)

func runSchema(args []string) int {
	cfg := config.Load(config.Options{})
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var list bool
	var outputFormat string

	fs.BoolVar(&list, "list", false, "list schema commands")
	fs.StringVar(&outputFormat, "output-format", "", "human or json")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	format := output.ResolveFormat(outputFormat, cfg.OutputFormat)

	if list {
		commands := []string{"render", "render-defaults"}
		if format == "json" {
			_ = output.PrintJSON(commands)
			return 0
		}

		for _, command := range commands {
			fmt.Fprintln(os.Stdout, command)
		}
		return 0
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "schema requires a target such as render or render.width")
		return 1
	}

	target := fs.Arg(0)
	name, field := splitSchemaTarget(target)

	manifest, err := schema.Load(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if field != "" {
		properties, ok := manifest["properties"].(map[string]interface{})
		if !ok {
			fmt.Fprintf(os.Stderr, "schema %s does not expose properties\n", name)
			return 1
		}

		fieldManifest, ok := properties[field]
		if !ok {
			fmt.Fprintf(os.Stderr, "schema field %s.%s not found\n", name, field)
			return 1
		}

		_ = output.PrintHuman(format, "", fieldManifest)
		return 0
	}

	_ = output.PrintHuman(format, "", manifest)
	return 0
}

func splitSchemaTarget(target string) (string, string) {
	parts := strings.SplitN(target, ".", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}

	return parts[0], parts[1]
}
