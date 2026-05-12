// internal/config/update.go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// lockAcquireTimeout caps how long Update will wait to acquire the
// config-file lock. 5s is far longer than any legitimate write should
// take (Save is a write + atomic rename) but bounded so a stale lock
// from a crashed process can't wedge the CLI indefinitely.
const lockAcquireTimeout = 5 * time.Second

// lockPollInterval is the back-off between Lock() attempts when the
// file is already locked by another process.
const lockPollInterval = 50 * time.Millisecond

// Update runs mutate against a freshly-loaded *Config and persists the
// result, holding an exclusive file lock for the duration. The lock
// closes the read-modify-write race Round 7 Adv-3 exercised: 20
// parallel config.profile.create calls used to silently lose ~6
// because each process loaded the same "before" state and the last
// writer's atomic rename clobbered the others.
//
// The lock is implemented as an O_CREAT|O_EXCL sentinel file
// alongside the config (config.json.lock). Cross-platform — no
// syscall.Flock, no third-party dependency. Stale locks from a
// crashed process expire when their owner's file handle GC'd; we
// also bound acquire time so a forever-wedged lock surfaces an
// error rather than hanging the CLI.
func Update(mutate func(*Config) error) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config dir: %w", err)
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

func acquireLock(lockPath string) error {
	deadline := time.Now().Add(lockAcquireTimeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) //nolint:gosec // lockPath is config.Path() + ".lock" — derived from the config-lookup path, not user input
		if err == nil {
			_ = f.Close()
			return nil
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("acquire config lock: %w", err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out acquiring config lock at %s after %s — remove the stale lockfile if no other urlbox process is running", lockPath, lockAcquireTimeout)
		}
		time.Sleep(lockPollInterval)
	}
}
