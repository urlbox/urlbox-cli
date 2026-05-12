// internal/config/update_test.go
package config_test

import (
	"fmt"
	"sync"
	"testing"

	"github.com/urlbox/urlbox-cli/internal/config"
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
