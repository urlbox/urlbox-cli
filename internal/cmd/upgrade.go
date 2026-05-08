package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/version"
)

// ExecFunc runs an external command, writing output to w.
type ExecFunc func(w io.Writer, name string, args ...string) error

// DetectInstallMethod determines how urlbox was installed based on the binary path.
func DetectInstallMethod(binaryPath string) string {
	path := strings.ToLower(binaryPath)

	if strings.Contains(path, "homebrew") || strings.Contains(path, "cellar") || strings.Contains(path, "/opt/homebrew/") || strings.Contains(path, "linuxbrew") {
		return "brew"
	}
	if strings.Contains(path, "scoop") {
		return "scoop"
	}
	if strings.Contains(path, "node_modules/@urlbox/cli") || strings.Contains(path, "node_modules\\@urlbox\\cli") {
		return "npm"
	}
	if strings.Contains(path, "/go/bin/") {
		return "go"
	}

	return "unknown"
}

// RunUpgrade executes the upgrade logic with the given binary path and command executor.
// This is exported to allow testing without shelling out to real package managers.
func RunUpgrade(stderr io.Writer, execPath string, runner ExecFunc) error {
	method := DetectInstallMethod(execPath)

	_, _ = fmt.Fprintf(stderr, "Current version: %s\n", version.Version)
	_, _ = fmt.Fprintf(stderr, "Install method: %s\n", method)
	_, _ = fmt.Fprintf(stderr, "Binary path: %s\n\n", execPath)

	switch method {
	case "brew":
		_, _ = fmt.Fprintln(stderr, "Upgrading via Homebrew...")
		return runner(stderr, "brew", "upgrade", "urlbox/tap/urlbox")
	case "scoop":
		_, _ = fmt.Fprintln(stderr, "Upgrading via Scoop...")
		return runner(stderr, "scoop", "update", "urlbox")
	case "npm":
		_, _ = fmt.Fprintln(stderr, "Upgrading via npm...")
		return runner(stderr, "npm", "install", "-g", "@urlbox/cli@latest")
	case "go":
		_, _ = fmt.Fprintln(stderr, "Upgrading via go install...")
		return runner(stderr, "go", "install", "github.com/urlbox/urlbox-cli/cmd/urlbox@latest")
	default:
		_, _ = fmt.Fprintln(stderr, "Could not detect install method.")
		_, _ = fmt.Fprintln(stderr, "To upgrade manually, run one of:")
		_, _ = fmt.Fprintln(stderr, "")
		_, _ = fmt.Fprintln(stderr, "  brew upgrade urlbox/tap/urlbox")
		_, _ = fmt.Fprintln(stderr, "  npm install -g @urlbox/cli@latest")
		_, _ = fmt.Fprintln(stderr, "  scoop update urlbox")
		_, _ = fmt.Fprintln(stderr, "  go install github.com/urlbox/urlbox-cli/cmd/urlbox@latest")
		_, _ = fmt.Fprintln(stderr, "  curl -fsSL https://cli.urlbox.com/install.sh | sh")
		return nil
	}
}

func newUpgradeCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Update urlbox to the latest version",
		Long:  "Detects how urlbox was installed and runs the appropriate update command.",
		RunE: func(cmd *cobra.Command, args []string) error {
			execPath, err := os.Executable()
			if err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"could not determine binary path: "+err.Error(),
					"This is a CLI bug — please report at https://github.com/urlbox/urlbox-cli/issues.",
				)
			}

			// Run the upgrade first — it streams human progress to stderr.
			// On failure we surface a CLIError; on success we emit the
			// standard envelope on stdout so agents get a structured
			// description of what happened.
			if err := RunUpgrade(cmd.ErrOrStderr(), execPath, runExternal); err != nil {
				return output.NewCLIError(
					output.ErrServer,
					"upgrade failed: "+err.Error(),
					"Re-run the package-manager command shown on stderr manually, or check https://github.com/urlbox/urlbox-cli/issues.",
				)
			}

			method := DetectInstallMethod(execPath)
			env := output.NewEnvelope("upgrade", map[string]any{
				"currentVersion": version.Version,
				"installMethod":  method,
				"binaryPath":     execPath,
			}, upgradeSummary(method), []output.Breadcrumb{
				{Action: "verify", Cmd: "urlbox --version"},
			})
			return writeEnvelope(cmd, env)
		},
	}
}

// upgradeSummary returns a human-friendly one-liner describing what the
// upgrade command just did, for the envelope's `summary` field.
func upgradeSummary(method string) string {
	switch method {
	case "brew":
		return "Upgraded via Homebrew"
	case "scoop":
		return "Upgraded via Scoop"
	case "npm":
		return "Upgraded via npm"
	case "go":
		return "Upgraded via go install"
	default:
		return "Manual upgrade instructions printed to stderr"
	}
}

func runExternal(w io.Writer, name string, args ...string) error {
	c := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // intentional: runs trusted package manager commands
	c.Stdout = w
	c.Stderr = w
	return c.Run()
}
