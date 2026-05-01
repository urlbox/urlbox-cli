package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// resolveOutputPath canonicalizes the user-supplied --output path and
// asserts it stays under baseDir (typically the CWD). Returns the
// absolute, cleaned path on success.
//
// Rejected (each pinned in tests):
//   - empty string
//   - contains NUL byte
//   - "../escape.png" (parent escape)
//   - absolute paths outside baseDir
//   - "~/whatever" outside baseDir (~ is expanded against $HOME)
//   - trailing slash (looks like a directory; --output must point at a file)
//
// Accepted:
//   - "out.png" → baseDir/out.png
//   - "./out.png" → baseDir/out.png
//   - "subdir/out.png" → baseDir/subdir/out.png (parent dir created on save)
//   - absolute paths under baseDir
func resolveOutputPath(userPath, baseDir string) (string, *output.CLIError) {
	if userPath == "" {
		return "", output.NewCLIError(
			output.ErrValidation,
			"--output path is empty",
			"Pass a file path: --output screenshot.png",
		)
	}
	if strings.ContainsRune(userPath, 0) {
		return "", output.NewCLIError(
			output.ErrValidation,
			"--output path contains a NUL byte",
			"Strip control characters from the path.",
		)
	}
	if strings.HasSuffix(userPath, "/") || strings.HasSuffix(userPath, string(os.PathSeparator)) {
		return "", output.NewCLIError(
			output.ErrValidation,
			"--output path ends in a separator (looks like a directory)",
			"Pass a file path, not a directory: --output screenshot.png",
		)
	}

	// Expand `~` against $HOME. Go's filepath doesn't do this; agents
	// commonly pass it.
	expanded := userPath
	if strings.HasPrefix(userPath, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(home, userPath[2:])
		}
	}

	// If the path is relative, anchor it on baseDir.
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(baseDir, expanded)
	}
	cleaned := filepath.Clean(expanded)

	// Sandbox check via filepath.Rel: if the result starts with `..`,
	// the cleaned path escapes baseDir.
	cleanedBase := filepath.Clean(baseDir)
	rel, err := filepath.Rel(cleanedBase, cleaned)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", output.NewCLIError(
			output.ErrValidation,
			fmt.Sprintf("--output path %q is outside the working directory", userPath),
			"Pass a path under the current directory, e.g. --output screenshot.png or --output renders/screenshot.png",
		)
	}
	return cleaned, nil
}

// downloadTo streams the body of url to dst. The parent directory is
// created (mode 0o755) if missing. Errors are wrapped as ErrNetwork
// (transport failures) or ErrServer (non-2xx response).
func downloadTo(ctx context.Context, url, dst string) *output.CLIError {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return output.NewCLIError(output.ErrUsage, "invalid render URL: "+err.Error(), "")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return output.NewCLIError(
			output.ErrNetwork,
			"failed to download render: "+err.Error(),
			"Check your internet connection. The render URL is in the envelope; you can curl it manually.",
		)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return output.NewCLIError(
			output.ErrServer,
			fmt.Sprintf("download returned HTTP %d", resp.StatusCode),
			"The render URL may have expired. Re-run urlbox render or check the dashboard.",
		)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return output.NewCLIError(output.ErrServer, "failed to create output directory: "+err.Error(), "")
	}

	// 0o644 (user rw, group/other r) is the right default for rendered
	// artifacts — users typically want to view them in a browser or
	// upload them to a webserver. Use the user's umask if they want
	// stricter perms; G302's 0o600 cap is too restrictive for content.
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // see comment above; rendered artifacts default to user-rw + world-readable
	if err != nil {
		return output.NewCLIError(output.ErrServer, "failed to create output file: "+err.Error(), "")
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return output.NewCLIError(output.ErrNetwork, "failed to write render to disk: "+err.Error(), "")
	}
	return nil
}
