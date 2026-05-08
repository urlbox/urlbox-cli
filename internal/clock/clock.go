// Package clock provides a tiny abstraction over time.Now / time.Sleep so
// time-sensitive code can be tested without real wall-clock waits.
//
// Production callers use clock.New() (a thin wrapper over the time package).
// Tests use clock.NewFake(start) and drive Now() forward with Advance(d). A
// goroutine inside FakeClock.Sleep blocks until enough Advance calls have
// pushed Now past its deadline, so a polling loop that takes minutes in
// production runs in microseconds in tests.
package clock

import (
	"sync"
	"time"
)

// Clock is the minimal abstraction over wall-clock time.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Sleep(d time.Duration)
}

// New returns a Clock backed by the real wall clock.
func New() Clock { return realClock{} }

type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }
func (realClock) Sleep(d time.Duration)           { time.Sleep(d) }

// FakeClock is a deterministic clock for tests. Its Now is set explicitly via
// NewFake / Advance. Sleep calls block until enough Advance has happened to
// move Now past the sleep deadline.
type FakeClock struct {
	mu        sync.Mutex
	now       time.Time
	sleepers  []*sleeper
	sleeperCh chan struct{} // signalled when a new sleeper registers
}

type sleeper struct {
	deadline time.Time
	released chan struct{}
}

// NewFake returns a FakeClock at the given start time.
func NewFake(start time.Time) *FakeClock {
	return &FakeClock{
		now:       start,
		sleeperCh: make(chan struct{}, 8),
	}
}

// Now returns the FakeClock's current synthetic time.
func (fc *FakeClock) Now() time.Time {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.now
}

// Since returns the duration that has elapsed (in synthetic time) since t.
func (fc *FakeClock) Since(t time.Time) time.Duration { return fc.Now().Sub(t) }

// Sleep blocks until Advance has moved the clock past the sleep deadline.
func (fc *FakeClock) Sleep(d time.Duration) {
	fc.mu.Lock()
	deadline := fc.now.Add(d)
	released := make(chan struct{})
	s := &sleeper{deadline: deadline, released: released}
	fc.sleepers = append(fc.sleepers, s)
	fc.mu.Unlock()

	// Notify any WaitForSleeper waiter. Non-blocking — channel is buffered.
	select {
	case fc.sleeperCh <- struct{}{}:
	default:
	}

	<-released
}

// Advance moves the clock forward by d and releases any sleepers whose
// deadlines are now reached.
func (fc *FakeClock) Advance(d time.Duration) {
	fc.mu.Lock()
	fc.now = fc.now.Add(d)
	now := fc.now
	remaining := fc.sleepers[:0]
	for _, s := range fc.sleepers {
		if !s.deadline.After(now) {
			close(s.released)
		} else {
			remaining = append(remaining, s)
		}
	}
	fc.sleepers = remaining
	fc.mu.Unlock()
}

// WaitForSleeper blocks until at least one goroutine is sleeping, or until d
// elapses. Returns true if a sleeper was observed, false on timeout. Used by
// tests that need to synchronise with a Sleep call before driving Advance.
func (fc *FakeClock) WaitForSleeper(d time.Duration) bool {
	// Fast-path: a sleeper already registered.
	fc.mu.Lock()
	if len(fc.sleepers) > 0 {
		fc.mu.Unlock()
		return true
	}
	fc.mu.Unlock()

	select {
	case <-fc.sleeperCh:
		return true
	case <-time.After(d):
		return false
	}
}
