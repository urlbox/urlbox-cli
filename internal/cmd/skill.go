package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/skills"
)

// supportedSkillTargets maps an agent-tooling target name to its skill-file
// destination resolver. Add new targets here; tests + the SKILL.md
// regression guard pin the matrix so changes are caught.
//
// `cursor`, `codex`, `opencode` are TBD — paths vary per release; not
// shipping until verified against current versions.
var supportedSkillTargets = map[string]skillTarget{
	"claude-code": {
		userPath:    func() (string, error) { return userHomeJoin(".claude", "skills", "urlbox", "SKILL.md") },
		projectPath: func() (string, error) { return cwdJoin(".claude", "skills", "urlbox", "SKILL.md") },
	},
}

type skillTarget struct {
	userPath    func() (string, error)
	projectPath func() (string, error)
}

func newSkillCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "skill",
		Short: "Agent skill content",
		Long:  "Subcommands for working with the embedded SKILL.md.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	parent.AddCommand(newSkillShowCmd())
	parent.AddCommand(newSkillInstallCmd())
	return parent
}

func newSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the embedded SKILL.md",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)

			if format == output.FormatText && jqExpr == "" {
				_, err := stdout.Write([]byte(skills.Content))
				return err
			}

			env := output.NewEnvelope(
				"skill show",
				map[string]string{"skill": skills.Content},
				"Embedded SKILL.md",
				nil,
			)
			if jqExpr != "" {
				return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			}
			styles := output.NewStylesForWriter(stdout)
			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	}
}

func newSkillInstallCmd() *cobra.Command {
	var target, scope string
	var yes bool
	c := &cobra.Command{
		Use:   "install",
		Short: "Install the embedded SKILL.md so your agent auto-discovers it",
		Long: `Install the urlbox SKILL.md into the well-known skill directory of
your agent tooling, so the agent picks it up automatically on next launch.

Currently supported targets:
  claude-code    Anthropic Claude Code (~/.claude/skills/urlbox/ or .claude/skills/urlbox/)

Scopes:
  user           Install once for all your projects (under $HOME).
  project        Install in the current repo (commits to git so teammates inherit).

Non-interactive (agents / CI):
  urlbox skill install --target claude-code --scope user --yes

Interactive (humans, on a TTY):
  urlbox skill install         # prompts for target + scope
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			stderr := cmd.ErrOrStderr()
			interactive := !yes && isStdinTTY(cmd.InOrStdin()) && isStderrTTY(stderr)

			resolvedTarget, err := resolveSkillTarget(target, interactive, stderr)
			if err != nil {
				return err
			}
			resolvedScope, err := resolveSkillScope(scope, interactive, stderr)
			if err != nil {
				return err
			}

			tgt := supportedSkillTargets[resolvedTarget]
			var pathFn func() (string, error)
			switch resolvedScope {
			case "user":
				pathFn = tgt.userPath
			case "project":
				pathFn = tgt.projectPath
			}
			path, err := pathFn()
			if err != nil {
				return output.NewCLIError(output.ErrServer, "could not resolve install path", err.Error())
			}

			// 0o700/0o600 — skill files live under the user's home / repo;
			// no need for world-read. gosec G301/G306 happy at these perms.
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return output.NewCLIError(output.ErrServer, "could not create skill directory", err.Error())
			}
			if err := os.WriteFile(path, []byte(skills.Content), 0o600); err != nil {
				return output.NewCLIError(output.ErrServer, "could not write skill file", err.Error())
			}

			env := output.NewEnvelope(
				"skill install",
				map[string]string{
					"target": resolvedTarget,
					"scope":  resolvedScope,
					"path":   path,
				},
				fmt.Sprintf("Installed urlbox skill for %s (%s scope) at %s", resolvedTarget, resolvedScope, path),
				[]output.Breadcrumb{
					{Action: "verify", Cmd: fmt.Sprintf("head %s", path)},
					{Action: "uninstall", Cmd: "rm -r " + filepath.Dir(path)},
				},
			)
			formatFlag, _ := cmd.Root().PersistentFlags().GetString("output-format")
			jqExpr, _ := cmd.Root().PersistentFlags().GetString("jq")
			stdout := cmd.OutOrStdout()
			format := output.ResolveFormat(formatFlag, stdout)
			styles := output.NewStylesForWriter(stdout)
			if jqExpr != "" {
				return output.WriteEnvelopeWithJQ(stdout, env, jqExpr, format == output.FormatQuiet)
			}
			formatter := output.NewFormatter(format, styles)
			return formatter.WriteSuccess(stdout, env)
		},
	}
	c.Flags().StringVar(&target, "target", "", "Agent tooling target (e.g. claude-code)")
	c.Flags().StringVar(&scope, "scope", "", "Install scope: user (global) or project (this repo)")
	c.Flags().BoolVar(&yes, "yes", false, "Skip interactive prompts (required when stdin/stderr aren't a TTY)")
	return c
}

func resolveSkillTarget(flag string, interactive bool, stderr io.Writer) (string, error) {
	if flag != "" {
		if _, ok := supportedSkillTargets[flag]; !ok {
			return "", output.NewCLIError(
				output.ErrUsage,
				fmt.Sprintf("unsupported target %q", flag),
				"Supported: "+listSupportedTargets()+". Cursor/Codex/OpenCode TBD.",
			)
		}
		return flag, nil
	}
	if !interactive {
		return "", output.NewCLIError(
			output.ErrUsage,
			"missing --target",
			"Pass --target <name> (e.g. --target claude-code), or run interactively on a TTY. Supported: "+listSupportedTargets()+".",
		)
	}
	_, _ = fmt.Fprintf(stderr, "Target [%s]: ", listSupportedTargets())
	v, err := readSkillLine()
	if err != nil {
		return "", output.NewCLIError(output.ErrUsage, "install cancelled", err.Error())
	}
	if _, ok := supportedSkillTargets[v]; !ok {
		return "", output.NewCLIError(
			output.ErrUsage,
			fmt.Sprintf("unsupported target %q", v),
			"Supported: "+listSupportedTargets()+".",
		)
	}
	return v, nil
}

func resolveSkillScope(flag string, interactive bool, stderr io.Writer) (string, error) {
	if flag == "user" || flag == "project" {
		return flag, nil
	}
	if flag != "" {
		return "", output.NewCLIError(
			output.ErrUsage,
			fmt.Sprintf("invalid scope %q", flag),
			"Pass --scope user (global, under $HOME) or --scope project (this repo, commits to git).",
		)
	}
	if !interactive {
		return "", output.NewCLIError(
			output.ErrUsage,
			"missing --scope",
			"Pass --scope user or --scope project, or run interactively on a TTY.",
		)
	}
	_, _ = fmt.Fprint(stderr, "Scope [user/project]: ")
	v, err := readSkillLine()
	if err != nil {
		return "", output.NewCLIError(output.ErrUsage, "install cancelled", err.Error())
	}
	if v != "user" && v != "project" {
		return "", output.NewCLIError(
			output.ErrUsage,
			fmt.Sprintf("invalid scope %q", v),
			"Answer 'user' (global) or 'project' (this repo).",
		)
	}
	return v, nil
}

func listSupportedTargets() string {
	out := ""
	for k := range supportedSkillTargets {
		if out != "" {
			out += ", "
		}
		out += k
	}
	return out
}

// userHomeJoin builds a path under $HOME. Errors if HOME is unset.
func userHomeJoin(parts ...string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{home}, parts...)...), nil
}

// cwdJoin builds a path under the current working directory.
func cwdJoin(parts ...string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{cwd}, parts...)...), nil
}

// readSkillLine reads one line from stdin and trims whitespace. Test-overridable.
var readSkillLine = func() (string, error) {
	var s string
	if _, err := fmt.Fscanln(os.Stdin, &s); err != nil {
		// EOF on empty line is a valid "no input" — treat as empty string.
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		return "", err
	}
	return s, nil
}
