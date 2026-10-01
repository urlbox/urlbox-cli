package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/urlbox/urlbox-cli/internal/output"
	"github.com/urlbox/urlbox-cli/internal/update"
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

	_, _ = fmt.Fprintf(stderr, "Current version: %s\n", updateCurrentVersion())
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
		Long: `Checks for a newer release, then detects how urlbox was installed and runs
the matching update command (brew, scoop, npm or go install). Does nothing
when you already have the latest release.

Exit codes:
  0   upgraded, already up to date, or manual instructions printed
  10  the package manager failed (its output is on stderr)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgradeCmd(cmd)
		},
	}
}

// upgradeCheckTimeout bounds the release lookup before an upgrade. Longer
// than the post-command notice's budget: the user asked for this one.
const upgradeCheckTimeout = 5 * time.Second

func runUpgradeCmd(cmd *cobra.Command) error {
	stderr := cmd.ErrOrStderr()
	execPath, err := upgradeExecPath()
	if err != nil {
		return output.NewCLIError(
			output.ErrServer,
			"could not determine binary path: "+err.Error(),
			"This is a CLI bug — please report at https://github.com/urlbox/urlbox-cli/issues.",
		)
	}
	// Package managers install a symlink on PATH (Intel Homebrew:
	// /usr/local/bin/urlbox -> ../Cellar/...). The real path is what
	// identifies the install method.
	if resolved, err := filepath.EvalSymlinks(execPath); err == nil {
		execPath = resolved
	}

	current := updateCurrentVersion()
	latest := latestForUpgrade(stderr, current)
	method := DetectInstallMethod(execPath)
	data := map[string]any{
		"currentVersion": current,
		"installMethod":  method,
		"binaryPath":     execPath,
	}
	breadcrumbs := []output.Breadcrumb{{Action: "verify", Cmd: "urlbox --version"}}
	if latest != "" {
		data["latestVersion"] = latest
		data["upToDate"] = !update.IsNewer(current, latest)
		if !update.IsNewer(current, latest) {
			_, _ = fmt.Fprintf(stderr, "urlbox %s is already up to date.\n", current)
			return writeEnvelope(cmd, output.NewEnvelope("upgrade", data,
				"Already up to date ("+current+")", breadcrumbs))
		}
		_, _ = fmt.Fprintf(stderr, "Upgrading urlbox %s → %s\n\n", current, latest)
	}

	// Run the upgrade first — it streams human progress to stderr.
	// On failure we surface a CLIError; on success we emit the
	// standard envelope on stdout so agents get a structured
	// description of what happened.
	if err := RunUpgrade(stderr, execPath, upgradeRunner); err != nil {
		return output.NewCLIError(
			output.ErrServer,
			"upgrade failed: "+err.Error(),
			"Re-run the package-manager command shown on stderr manually, or check https://github.com/urlbox/urlbox-cli/issues.",
		)
	}
	return writeEnvelope(cmd, output.NewEnvelope("upgrade", data,
		upgradeSummary(method, current, latest), breadcrumbs))
}

// latestForUpgrade looks up the newest release. Returns "" for local dev
// builds (nothing to compare) and when the lookup fails — a failed check
// never blocks the upgrade itself.
func latestForUpgrade(stderr io.Writer, current string) string {
	if !update.IsRelease(current) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), upgradeCheckTimeout)
	defer cancel()
	latest, err := updateFetchLatest(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Couldn't check for the latest version (%v); upgrading anyway.\n\n", err)
		return ""
	}
	return latest
}

// upgradeSummary returns a human-friendly one-liner describing what the
// upgrade command just did, for the envelope's `summary` field.
func upgradeSummary(method, current, latest string) string {
	via := map[string]string{
		"brew":  "Homebrew",
		"scoop": "Scoop",
		"npm":   "npm",
		"go":    "go install",
	}[method]
	switch {
	case via == "":
		return "Manual upgrade instructions printed to stderr"
	case latest != "":
		return fmt.Sprintf("Upgraded %s → %s via %s", current, latest, via)
	default:
		return "Upgraded via " + via
	}
}

func runExternal(w io.Writer, name string, args ...string) error {
	c := exec.CommandContext(context.Background(), name, args...) //nolint:gosec // intentional: runs trusted package manager commands
	c.Stdout = w
	c.Stderr = w
	return c.Run()
}

// SetUpgradeForTest pins the binary path and package-manager runner.
// Pair with t.Cleanup(ResetUpgradeForTest).
func SetUpgradeForTest(execPath string, runner ExecFunc) {
	upgradeExecPath = func() (string, error) { return execPath, nil }
	upgradeRunner = runner
}

// ResetUpgradeForTest restores os.Executable and the real runner.
func ResetUpgradeForTest() {
	upgradeExecPath = os.Executable
	upgradeRunner = runExternal
}

var (
	upgradeExecPath          = os.Executable
	upgradeRunner   ExecFunc = runExternal
)
