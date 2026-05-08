// Package clock_test exercises the realClock + FakeClock contract.
package clock_test

import (
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/clock"
)

// TestRealClock_Now_IsRecent confirms New() returns a clock whose Now() is
// (approximately) wall-clock now. We allow a 1s slack so this test is robust
// on slow CI workers.
func TestRealClock_Now_IsRecent(t *testing.T) {
	c := clock.New()
	if time.Since(c.Now()) > time.Second {
		t.Fatal("clock.Now() not recent")
	}
}

// TestFakeClock_AdvanceMovesNow confirms NewFake's Now() reports the start
// time, and Advance(d) shifts Now by exactly d.
func TestFakeClock_AdvanceMovesNow(t *testing.T) {
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	fc := clock.NewFake(start)
	if !fc.Now().Equal(start) {
		t.Fatalf("expected %v, got %v", start, fc.Now())
	}
	fc.Advance(5 * time.Second)
	if !fc.Now().Equal(start.Add(5 * time.Second)) {
		t.Fatalf("after Advance(5s), expected +5s, got %v", fc.Now())
	}
}

// TestFakeClock_SleepReleasesAfterAdvance confirms a goroutine inside Sleep(d)
// stays parked until Advance moves Now past the sleep deadline.
func TestFakeClock_SleepReleasesAfterAdvance(t *testing.T) {
	fc := clock.NewFake(time.Now())
	done := make(chan struct{})
	go func() {
		fc.Sleep(2 * time.Second)
		close(done)
	}()
	if !fc.WaitForSleeper(time.Second) {
		t.Fatal("no sleeper registered")
	}
	select {
	case <-done:
		t.Fatal("Sleep returned before clock advanced")
	case <-time.After(20 * time.Millisecond):
	}
	fc.Advance(2 * time.Second)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Sleep did not return after Advance")
	}
}
