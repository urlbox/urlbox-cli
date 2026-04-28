// internal/output/help.go
package output

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// AgentHelp is the structured help payload for the --agent flag.
type AgentHelp struct {
	Name        string        `json:"name"`
	Usage       string        `json:"usage"`
	Description string        `json:"description,omitempty"`
	Flags       []AgentFlag   `json:"flags,omitempty"`
	Subcommands []AgentSubCmd `json:"subcommands,omitempty"`
	Examples    []string      `json:"examples,omitempty"`
}

// AgentFlag describes one flag.
type AgentFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
}

// AgentSubCmd describes one subcommand.
type AgentSubCmd struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// BuildAgentHelp returns a JSON envelope describing the command for agents.
func BuildAgentHelp(cmd *cobra.Command) *Envelope {
	help := AgentHelp{
		Name:        cmd.CommandPath(),
		Usage:       cmd.UseLine(),
		Description: cmd.Long,
	}
	if help.Description == "" {
		help.Description = cmd.Short
	}

	seen := map[string]bool{}
	collect := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(f *pflag.Flag) {
			if f.Hidden || seen[f.Name] {
				return
			}
			seen[f.Name] = true
			help.Flags = append(help.Flags, AgentFlag{
				Name:        f.Name,
				Shorthand:   f.Shorthand,
				Type:        f.Value.Type(),
				Description: f.Usage,
				Default:     f.DefValue,
			})
		})
	}
	collect(cmd.PersistentFlags())
	collect(cmd.LocalFlags())
	for p := cmd.Parent(); p != nil; p = p.Parent() {
		collect(p.PersistentFlags())
	}

	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		help.Subcommands = append(help.Subcommands, AgentSubCmd{
			Name:        sub.Name(),
			Description: sub.Short,
		})
	}

	if ex, ok := cmd.Annotations["agent_examples"]; ok && ex != "" {
		help.Examples = []string{ex}
	}

	return NewEnvelope(
		cmd.CommandPath(),
		help,
		"Help for "+cmd.CommandPath(),
		[]Breadcrumb{
			{Action: "list", Cmd: "urlbox commands --output-format json"},
		},
	)
}
