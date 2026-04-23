package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/urlbox/cli/internal/version"
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
	case "go":
		_, _ = fmt.Fprintln(stderr, "Upgrading via go install...")
		return runner(stderr, "go", "install", "github.com/urlbox/cli/cmd/urlbox@latest")
	default:
		_, _ = fmt.Fprintln(stderr, "Could not detect install method.")
		_, _ = fmt.Fprintln(stderr, "To upgrade manually, run one of:")
		_, _ = fmt.Fprintln(stderr, "")
		_, _ = fmt.Fprintln(stderr, "  brew upgrade urlbox/tap/urlbox")
		_, _ = fmt.Fprintln(stderr, "  scoop update urlbox")
		_, _ = fmt.Fprintln(stderr, "  go install github.com/urlbox/cli/cmd/urlbox@latest")
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
				return fmt.Errorf("could not determine binary path: %w", err)
			}
			return RunUpgrade(stderr, execPath, runExternal)
		},
	}
}

func runExternal(w io.Writer, name string, args ...string) error {
	c := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // intentional: runs trusted package manager commands
	c.Stdout = w
	c.Stderr = w
	return c.Run()
}
