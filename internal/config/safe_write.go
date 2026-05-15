// internal/config/safe_write.go — v1.0.4 Class 4.
//
// SafeWriteUserFile is the single helper every CLI-initiated write to
// a user-owned path goes through. The contract:
//
//   - Lstat first. If the target resolves to a symlink, refuse. Without
//     this, an attacker who plants `~/.claude/skills/urlbox/SKILL.md ->
//     /etc/something` turns a `urlbox skill install` re-run into a
//     write-anywhere primitive.
//   - Refuse to overwrite an existing regular file unless opts.Force.
//     Pre-v1.0.4 `skill install` used bare os.WriteFile, silently
//     destroying user-edited skill files on re-run.
//   - Atomic rename: write to a sibling temp file with the target's
//     permissions, then os.Rename into place. If the write fails
//     halfway, no partial file is visible.
//   - Parent dirs created with opts.DirMode (default 0o700).
//   - File mode opts.Mode (default 0o600).
//
// Errors are sentinel values (ErrSafeWriteSymlink, ErrSafeWriteExists)
// + wrapped filesystem errors. Callers wrap into *output.CLIError with
// the appropriate Code at the cmd boundary; this file stays free of
// the output dependency so any future config-package writer can reuse
// the helper without an import cycle.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrSafeWriteSymlink is returned when the target path resolves to a
// symlink. Caller-side cmd code maps this to ErrUsage (the user owns
// the path; the CLI refuses to follow it as a safety measure).
var ErrSafeWriteSymlink = errors.New("refusing to follow symlink at target path")

// ErrSafeWriteExists is returned when the target path exists as a
// regular file and Force is false. Caller-side cmd code maps this to
// ErrConflict and surfaces a "--force to overwrite" hint.
var ErrSafeWriteExists = errors.New("file already exists at target path")

// SafeWriteOptions controls SafeWriteUserFile's overwrite + permission policy.
type SafeWriteOptions struct {
	// Force allows overwriting an existing regular file. Without it, an
	// existing file at path causes SafeWriteUserFile to return
	// ErrSafeWriteExists. Symlinks are refused regardless of Force —
	// the symlink rule is a security invariant, not a UX preference.
	Force bool

	// Mode is the file permission for the written content. Zero defaults
	// to 0o600 (user-only) because the helper's callers (skill install,
	// future config writers) handle sensitive content.
	Mode os.FileMode

	// DirMode is the permission used by os.MkdirAll for the parent.
	// Zero defaults to 0o700 (user-only).
	DirMode os.FileMode
}

// SafeWriteUserFile writes content to path under the invariants
// documented at the package level (Lstat refuses symlinks, no clobber
// without Force, atomic rename, default 0600/0700 perms).
//
// Errors:
//   - ErrSafeWriteSymlink if path resolves to a symlink.
//   - ErrSafeWriteExists if path is an existing regular file and
//     opts.Force is false.
//   - Wrapped filesystem errors for stat / mkdir / write / rename
//     failures (use errors.Unwrap to inspect).
func SafeWriteUserFile(path string, content []byte, opts SafeWriteOptions) error {
	if opts.Mode == 0 {
		opts.Mode = 0o600
	}
	if opts.DirMode == 0 {
		opts.DirMode = 0o700
	}

	// Lstat first — refuse symlinks regardless of Force.
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrSafeWriteSymlink, path)
		}
		if !opts.Force && info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s (pass --force to overwrite)", ErrSafeWriteExists, path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", path, err)
	}

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, opts.DirMode); err != nil {
		return fmt.Errorf("create parent dir %s: %w", parent, err)
	}

	// Atomic rename via sibling temp file. CreateTemp generates a unique
	// name and opens with 0o600 by default — we Chmod up/down to opts.Mode
	// before closing so the final file lands at the requested mode.
	tmp, err := os.CreateTemp(parent, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	// Clean up the temp on any error path (no-op on success since rename
	// removed it).
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(opts.Mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpName, path, err)
	}
	return nil
}
