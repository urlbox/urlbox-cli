// internal/cmd/overlay.go — Round 8 Class C (HH): wires up the
// per-repo overlay loader. Before this commit, internal/config/repo.go
// defined LoadRepoOverlay and internal/config/resolve.go handled the
// "repo" precedence slot, but NO command actually called the loader.
// README + SKILL.md + CHANGELOG all advertised `.urlbox/config.json`
// support, while in reality the file was silently ignored — Round 8
// Adv-4 found this by setting an overlay and seeing render use the
// global default instead.
package cmd

import (
	"os"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// loadRepoOverlay walks from the working directory up to (but not
// including) $HOME looking for .urlbox/config.json. Returns:
//
//   - (overlay, nil) if a valid overlay was found and parsed
//   - (nil, nil) if no overlay was found before crossing the boundary
//     (or if CWD can't be determined — overlay is optional)
//   - (nil, *CLIError ErrUsage) if a file was found but couldn't be
//     parsed. The error message names the offending path so the user
//     can find and fix it.
func loadRepoOverlay() (*config.RepoOverlay, *output.CLIError) {
	cwd, err := os.Getwd()
	if err != nil {
		// Rare (e.g. CWD deleted). Treat as "no overlay" rather than
		// failing the whole command — the overlay is an optional layer.
		return nil, nil
	}
	home, _ := os.UserHomeDir() // empty home = scan to filesystem root, OK
	overlay, err := config.LoadRepoOverlay(cwd, home)
	if err != nil {
		return nil, output.NewCLIError(
			output.ErrUsage,
			"malformed repo overlay: "+err.Error(),
			"Fix the JSON in the .urlbox/config.json named above, or remove the file. The CLI walks up from CWD looking for this overlay (stops at $HOME).",
		)
	}
	return overlay, nil
}
