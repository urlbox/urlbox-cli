package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// downloadMaxDuration caps how long a single render-bytes download can
// run. Server-side render time + retries are budgeted by the API call's
// context; this is just the streaming-read leg.
const downloadMaxDuration = 5 * time.Minute

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

	// Canonicalize baseDir up front (resolves macOS /var → /private/var,
	// any symlinks in the chain) so the rest of the function works in
	// canonical space.
	canonicalBase := filepath.Clean(baseDir)
	if resolved, err := filepath.EvalSymlinks(canonicalBase); err == nil {
		canonicalBase = resolved
	}

	// Expand `~` and `~/...` against $HOME. Go's filepath doesn't do this;
	// agents commonly pass it. Bare `~` resolves to $HOME (not a literal
	// filename "~", which would surprise users).
	expanded := userPath
	switch {
	case userPath == "~":
		if home, err := os.UserHomeDir(); err == nil {
			expanded = home
		}
	case strings.HasPrefix(userPath, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, userPath[2:])
		}
	}

	// Anchor relative paths on the *canonical* base so the sandbox check
	// compares apples to apples.
	if !filepath.IsAbs(expanded) {
		expanded = filepath.Join(canonicalBase, expanded)
	}
	cleaned := filepath.Clean(expanded)

	// EvalSymlinks the deepest existing ancestor so a planted symlink in
	// baseDir can't redirect the write outside. Walk up until we find an
	// existing dir (the user's --output may target a file in a directory
	// that doesn't exist yet — we'll mkdir it on save).
	cleaned = canonicalizeExistingPrefix(cleaned)

	// Sandbox check via filepath.Rel: if the result starts with `..`, the
	// cleaned path escapes baseDir.
	rel, err := filepath.Rel(canonicalBase, cleaned)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", output.NewCLIError(
			output.ErrValidation,
			fmt.Sprintf("--output path %q is outside the working directory", userPath),
			"Pass a path under the current directory, e.g. --output screenshot.png or --output renders/screenshot.png",
		)
	}
	return cleaned, nil
}

// canonicalizeExistingPrefix walks up `path` until it finds an existing
// directory, EvalSymlinks that directory, and reattaches the unresolved
// suffix. Defends against symlink-based sandbox escapes where an attacker
// plants `link → /etc` in baseDir and the user passes `--output link/foo.png`.
//
// If no ancestor exists, returns the cleaned input unchanged — the caller's
// sandbox check is the final line of defense.
//
// Note: this defends against parent-symlinks. The leaf is defended
// separately by the Lstat probe in downloadTo before open.
func canonicalizeExistingPrefix(path string) string {
	suffix := ""
	current := path
	for {
		if info, err := os.Stat(current); err == nil && info.IsDir() {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return path
			}
			if suffix == "" {
				return resolved
			}
			return filepath.Join(resolved, suffix)
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root without finding an existing ancestor.
			return path
		}
		if suffix == "" {
			suffix = filepath.Base(current)
		} else {
			suffix = filepath.Join(filepath.Base(current), suffix)
		}
		current = parent
	}
}

// downloadTo streams the body of url to dst. The parent directory is
// created (mode 0o750) if missing. Errors are wrapped as ErrNetwork
// (transport failures) or ErrServer (non-2xx response).
//
// The download is bounded by downloadMaxDuration even if the caller's
// context has no deadline — a stalled CDN connection mustn't hang the
// CLI indefinitely.
func downloadTo(ctx context.Context, url, dst string) *output.CLIError {
	dlCtx, cancel := context.WithTimeout(ctx, downloadMaxDuration)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, http.NoBody)
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

	// Reject leaf symlinks before opening the file: O_NOFOLLOW would do
	// this kernel-side but isn't portable across the OSes we ship for.
	// Lstat probes the path itself (without following the final link) and
	// rejects if it resolves to a symlink. Defends against an attacker
	// planting `out.png → /etc/hosts` and waiting for `urlbox render
	// --output out.png` to overwrite it.
	if info, err := os.Lstat(dst); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return output.NewCLIError(
			output.ErrValidation,
			"--output path is an existing symlink; refusing to follow",
			"Remove the symlink at "+dst+" first, or write to a different path.",
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
