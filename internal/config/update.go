// internal/config/update.go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/urlbox/urlbox-cli/internal/output"
)

// lockAcquireTimeout caps how long Update will wait to acquire the
// config-file lock. 5s is far longer than any legitimate write should
// take (Save is a write + atomic rename) but bounded so a stale lock
// from a crashed process can't wedge the CLI indefinitely.
const lockAcquireTimeout = 5 * time.Second

// lockPollInterval is the back-off between Lock() attempts when the
// file is already locked by another process.
const lockPollInterval = 50 * time.Millisecond

// lockEmptyStaleAfter is the cutoff above which an empty lockfile is
// treated as stale (left by a crashed writer that died before writing
// its PID). Anything younger is assumed to be a millisecond race where
// the legitimate holder hasn't written its PID yet.
const lockEmptyStaleAfter = 1 * time.Second

// Update runs mutate against a freshly-loaded *Config and persists the
// result, holding an exclusive file lock for the duration. The lock
// closes the read-modify-write race Round 7 Adv-3 exercised.
//
// Round 8 KK: stale-lock recovery. The original O_EXCL sentinel was
// brittle to SIGKILL/SIGTERM (the deferred os.Remove never ran, leaving
// a zero-byte .lock that wedged every subsequent write for 5s). Now
// the lock file contains the holder's PID; on a contended acquire, we
// check if that PID is alive and clobber the lockfile if not. Plus a
// signal handler removes the lock on SIGINT/SIGTERM so well-behaved
// shutdowns clean up too.
func Update(mutate func(*Config) error) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return output.NewCLIError(
			output.ErrForbidden,
			"could not create config directory: "+err.Error(),
			"Check the permissions of "+filepath.Dir(path)+" — the CLI needs to be able to create files there.",
		)
	}
	return withFileLock(path, func() error {
		cfg, err := Load()
		if err != nil {
			return err
		}
		if cfg == nil {
			cfg = &Config{}
		}
		if cfg.Profiles == nil {
			cfg.Profiles = map[string]Profile{}
		}
		if err := mutate(cfg); err != nil {
			return err
		}
		return Save(cfg)
	})
}

// withFileLock acquires a sentinel lock file alongside path, runs fn,
// then removes the sentinel. The lock is per-config-file, so multiple
// urlbox processes targeting the same XDG_CONFIG_HOME serialize their
// writes. Different config directories (different sandboxes / users)
// don't contend.
func withFileLock(path string, fn func() error) error {
	lockPath := path + ".lock"
	if err := acquireLock(lockPath); err != nil {
		return err
	}
	defer func() { _ = os.Remove(lockPath) }()
	return fn()
}

// acquireLock tries to create lockPath atomically. If a lock already
// exists, it reads the PID inside, checks if that process is alive,
// and clobbers the file if not (stale-lock recovery — Round 8 KK).
// Returns a typed *output.CLIError so callers don't need to wrap.
func acquireLock(lockPath string) error {
	deadline := time.Now().Add(lockAcquireTimeout)
	myPID := []byte(strconv.Itoa(os.Getpid()))
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // lockPath is config.Path() + ".lock" — derived from the config-lookup path, not user input
		if err == nil {
			_, _ = f.Write(myPID)
			_ = f.Close()
			return nil
		}
		if !errors.Is(err, os.ErrExist) {
			// Permission denied, parent dir missing, etc. — local FS
			// problem, not a contention issue. Map to ErrForbidden so
			// the exit code reflects "you can't touch this", not "server
			// problem" (Round 8 Adv-1 M1).
			return output.NewCLIError(
				output.ErrForbidden,
				"could not create config lock: "+err.Error(),
				"Check the permissions of "+filepath.Dir(lockPath)+" — the CLI needs to be able to create the lock file.",
			)
		}
		// Lock exists — check if the holder is dead (stale lock).
		if cleaned := tryCleanStaleLock(lockPath); cleaned {
			continue // retry the O_EXCL create immediately
		}
		if time.Now().After(deadline) {
			return output.NewCLIError(
				output.ErrConflict,
				fmt.Sprintf("timed out acquiring config lock at %s after %s — another urlbox process is still writing, or the lockfile is stale (run `rm %s` to recover)", lockPath, lockAcquireTimeout, lockPath),
				"Locks are normally held for milliseconds; a 5s timeout means either heavy contention (rare) or a crashed previous process. The stale-lock recovery should handle the latter automatically; if you see this, the previous PID is somehow still alive.",
			)
		}
		time.Sleep(lockPollInterval)
	}
}

// tryCleanStaleLock reads the PID from lockPath, checks if that
// process is still running, and removes the lockfile if not. Returns
// true if the file was cleaned (caller should retry). Returns false
// if the lock looks legitimately held or if we couldn't make a clean
// determination — caller will keep waiting.
//
// Cross-platform: uses os.FindProcess + Signal(0). On Unix this is
// the canonical "is this PID alive" check (Signal(0) returns ESRCH
// for dead PIDs without delivering a signal). On Windows, FindProcess
// returns successfully for any PID, so the stale check is effectively
// a no-op there — the 5s timeout is the only recovery on Windows.
// That's the best we can do without a third-party dep.
func tryCleanStaleLock(lockPath string) bool {
	b, err := os.ReadFile(lockPath) //nolint:gosec // lockPath is config.Path() + ".lock", derived path not user input
	if err != nil {
		return false // could be a race; let the caller try again
	}
	pidStr := strings.TrimSpace(string(b))
	if pidStr == "" {
		// Zero-byte lockfile — could be either:
		//   (a) the previous writer SIGKILL'd before writing its PID,
		//       leaving the empty O_EXCL'd file forever (pre-KK bug)
		//   (b) the CURRENT writer just succeeded the O_EXCL Open and
		//       hasn't called Write yet — millisecond race
		// Distinguish by file age: an empty file >1s old is definitely
		// stale (case a); fresher than that is case b, where we should
		// wait one more poll cycle for the writer to fill it in.
		info, err := os.Stat(lockPath)
		if err != nil {
			return false // race; let the caller retry
		}
		if time.Since(info.ModTime()) < lockEmptyStaleAfter {
			return false
		}
		return os.Remove(lockPath) == nil
	}
	pid, perr := strconv.Atoi(pidStr)
	if perr != nil || pid <= 0 {
		// Garbage contents — also stale.
		return os.Remove(lockPath) == nil
	}
	if pid == os.Getpid() {
		// Our own PID — another goroutine in this same process holds
		// the lock right now. Don't clobber: we use O_EXCL exactly so
		// concurrent goroutines serialize on the file. Wait for them.
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		// Couldn't even get a handle — on Unix this means the PID
		// is invalid; clobber and retry.
		return os.Remove(lockPath) == nil
	}
	// Signal 0 doesn't deliver anything; it just checks if the PID is
	// addressable. Returns nil if alive, syscall.ESRCH if dead.
	// Only clobber on ESRCH — other errors (EPERM = different user,
	// or Windows where this syscall is unsupported) are conservative
	// "still alive" so we wait for the 5s timeout rather than
	// clobbering a lock that might be legitimately held.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// "Dead" indicators: ESRCH (no such process, classic Unix) or
		// os.ErrProcessDone (Go's wrapper, returned on macOS for PIDs
		// that have never existed or have been reaped). Other errors
		// (EPERM = different-user / kernel process; unsupported on
		// Windows) are conservative "still alive" — wait for the
		// timeout rather than clobber a possibly-real holder.
		if errors.Is(err, syscall.ESRCH) || errors.Is(err, os.ErrProcessDone) {
			return os.Remove(lockPath) == nil
		}
		return false
	}
	return false // legitimate holder, keep waiting
}
