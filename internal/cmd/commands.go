// internal/cmd/commands.go
package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// CommandInfo describes a command for the catalog.
type CommandInfo struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Aliases     []string   `json:"aliases,omitempty"`
	Flags       []FlagInfo `json:"flags,omitempty"`
}

// FlagInfo describes a flag for the catalog.
type FlagInfo struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

// Catalog is the data payload for the commands command.
type Catalog struct {
	Commands    []CommandInfo `json:"commands"`
	GlobalFlags []FlagInfo    `json:"global_flags"`
}

func newCommandsCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "commands",
		Short: "List all available commands",
		Long:  "Outputs the full command catalog. Use --output-format json for agent-parseable output.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			format := output.ResolveFormat(formatFlag, stdout)
			styles := output.NewStylesForWriter(stdout)

			catalog := buildCatalog(cmd.Root())
			summary := fmt.Sprintf("%d commands available", len(catalog.Commands))

			env := output.NewEnvelope("commands", catalog, summary, []output.Breadcrumb{
				{Action: "help", Cmd: "urlbox <command> --help"},
			})

			if format == output.FormatText {
				return writeCommandsText(stdout, styles, catalog)
			}

			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	}
}

func buildCatalog(root *cobra.Command) *Catalog {
	var commands []CommandInfo
	for _, c := range root.Commands() {
		if c.Hidden || c.Name() == "help" {
			continue
		}
		commands = append(commands, buildCommandInfo(c))
	}

	var globalFlags []FlagInfo
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		globalFlags = append(globalFlags, buildFlagInfo(f))
	})

	return &Catalog{
		Commands:    commands,
		GlobalFlags: globalFlags,
	}
}

func buildCommandInfo(cmd *cobra.Command) CommandInfo {
	info := CommandInfo{
		Name:        cmd.Name(),
		Description: cmd.Short,
		Aliases:     cmd.Aliases,
	}

	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		info.Flags = append(info.Flags, buildFlagInfo(f))
	})

	return info
}

func buildFlagInfo(f *pflag.Flag) FlagInfo {
	info := FlagInfo{
		Name:        f.Name,
		Shorthand:   f.Shorthand,
		Type:        f.Value.Type(),
		Description: f.Usage,
	}
	if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
		info.Default = f.DefValue
	}
	return info
}

func writeCommandsText(w io.Writer, styles *output.Styles, catalog *Catalog) error {
	_, _ = fmt.Fprintln(w, styles.Bold.Render("Available commands:"))
	_, _ = fmt.Fprintln(w)

	maxLen := 0
	for _, c := range catalog.Commands {
		if len(c.Name) > maxLen {
			maxLen = len(c.Name)
		}
	}

	for _, c := range catalog.Commands {
		_, _ = fmt.Fprintf(w, "  %-*s  %s\n", maxLen, styles.Bold.Render(c.Name), c.Description)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, styles.Muted.Render("Use \"urlbox <command> --help\" for more information about a command."))
	return nil
}
