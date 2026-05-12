// internal/config/update_test.go
package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/config"
	"github.com/urlbox/urlbox-cli/internal/output"
)

// TestUpdate_ConcurrentProfileCreates_AllPersist pins Round 7 Adv-3 (High):
// 20 goroutines each calling config.Update to add a distinct profile must
// all persist. Before the lock, the Load -> mutate -> Save sequence raced —
// processes started from the same "before" state and silently overwrote
// each other's writes via the atomic temp-file rename in Save. The
// goroutine test exercises the same OS-level race because each Update call
// opens its own file handles and goes through the same Load/Save path.
func TestUpdate_ConcurrentProfileCreates_AllPersist(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("p%02d", i)
			secret := fmt.Sprintf("sec_test_%02d_xxxxxxx", i)
			err := config.Update(func(c *config.Config) error {
				if c.Profiles == nil {
					c.Profiles = map[string]config.Profile{}
				}
				c.Profiles[name] = config.Profile{APISecret: secret}
				return nil
			})
			if err != nil {
				t.Errorf("Update(p%02d) errored: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(got.Profiles) != n {
		t.Errorf("expected %d profiles to persist; got %d (%v)", n, len(got.Profiles), got.Profiles)
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("p%02d", i)
		if p, ok := got.Profiles[name]; !ok {
			t.Errorf("profile %s missing", name)
		} else if p.APISecret != fmt.Sprintf("sec_test_%02d_xxxxxxx", i) {
			t.Errorf("profile %s has wrong secret: %s", name, p.APISecret)
		}
	}
}

// TestUpdate_MutateFnErrorPropagates pins that an error from the user's
// mutate fn surfaces verbatim — the lock helper doesn't swallow it.
func TestUpdate_MutateFnErrorPropagates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	sentinel := fmt.Errorf("mutate sentinel")
	err := config.Update(func(c *config.Config) error {
		return sentinel
	})
	if err == nil || err.Error() != sentinel.Error() {
		t.Errorf("expected sentinel error to propagate; got %v", err)
	}
}

// TestUpdate_StaleLock_AfterKill_SelfHeals pins Round 8 KK: the pre-KK
// O_EXCL approach left a 0-byte .lock file after SIGKILL of the
// previous writer, wedging every subsequent write for 5s. Now we
// detect dead-PID/zero-byte lock files and clobber them on the spot.
func TestUpdate_StaleLock_AfterKill_SelfHeals(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Simulate the SIGKILL-left state: write a zero-byte lockfile
	// alongside the config (mimics what the old O_EXCL path left
	// behind when the process died before its deferred remove ran).
	cfgPath := config.Path()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lockPath := cfgPath + ".lock"
	if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}
	// Backdate the mtime to simulate a SIGKILL'd previous process: an
	// empty lockfile minutes old is definitely stale (not a fresh
	// in-flight write). Without this backdate, the "is this a fresh
	// O_EXCL'd file mid-write or a stale leftover?" heuristic would
	// wait the full 1s grace window.
	past := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatalf("backdate lock: %v", err)
	}

	// Update should detect the empty lock as stale and proceed
	// near-instantly — NOT wait 5s for the timeout.
	start := time.Now()
	err := config.Update(func(c *config.Config) error {
		c.Profiles["test"] = config.Profile{APISecret: "sec_post_recovery"}
		return nil
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Update should recover from stale lock; got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("recovery took %v — should be near-instant once stale lock detected", elapsed)
	}
}

// TestUpdate_StaleLock_DeadPID_SelfHeals exercises the case where the
// crashed writer DID write its PID before dying. Same recovery path.
func TestUpdate_StaleLock_DeadPID_SelfHeals(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgPath := config.Path()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lockPath := cfgPath + ".lock"
	// PID 2^30 is virtually guaranteed to be unallocated on any sane
	// system. Test would race only if the OS recycles to this exact
	// number — extraordinarily unlikely.
	deadPID := "1073741824"
	if err := os.WriteFile(lockPath, []byte(deadPID), 0o600); err != nil {
		t.Fatalf("seed dead-pid lock: %v", err)
	}

	start := time.Now()
	err := config.Update(func(c *config.Config) error {
		c.Profiles["test2"] = config.Profile{APISecret: "sec_after_dead_pid"}
		return nil
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Update should recover from dead-PID lock; got %v", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("recovery took %v — dead-PID should be detected fast", elapsed)
	}
}

// TestUpdate_LockAcquireTimeout_ReturnsConflictNotServer pins Round 8
// KK / Adv-1 M1: local-IO / lock-contention errors used to surface as
// code:"server" (exit 10), which the contract reserves for upstream
// server problems. Now they're code:"conflict" (exit 7).
func TestUpdate_LockAcquireTimeout_ReturnsConflictNotServer(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfgPath := config.Path()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lockPath := cfgPath + ".lock"
	// Hold the lock with OUR pid so the stale-check sees it as alive.
	pid := []byte(strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(lockPath, pid, 0o600); err != nil {
		t.Fatalf("seed alive-pid lock: %v", err)
	}

	// Run from a goroutine that releases after the timeout would expire,
	// just to make sure we don't actually wait the full lockAcquireTimeout
	// in CI. But the test expects the call to timeout, since our process
	// IS alive (the stale-check sees us as live holder).
	//
	// HOWEVER: the new stale check special-cases pid == os.Getpid() (treats
	// as stale because we wouldn't be acquiring again if we held it). So
	// this test instead writes a different alive PID — its own parent.
	parentPID := os.Getppid()
	if err := os.WriteFile(lockPath, []byte(strconv.Itoa(parentPID)), 0o600); err != nil {
		t.Fatalf("seed parent-pid lock: %v", err)
	}
	_ = pid

	// Now Update should wait for the full timeout (since parent is alive)
	// and return an ErrConflict, not ErrServer.
	err := config.Update(func(c *config.Config) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	var cli *output.CLIError
	if !errors.As(err, &cli) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cli.Code != output.ErrConflict {
		t.Errorf("code=%q, want conflict (exit 7), not server (10)", cli.Code)
	}
}
