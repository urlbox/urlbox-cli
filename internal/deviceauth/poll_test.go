package deviceauth

import (
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/clock"
	"github.com/urlbox/urlbox-cli/internal/output"
)

func runPoll(t *testing.T, interval, expiresIn int, script []Exchange) (string, *output.CLIError, *clock.FakeClock) {
	t.Helper()
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	i := 0
	exchange := func() Exchange {
		if i >= len(script) {
			t.Fatalf("poll exceeded script (%d calls)", i)
		}
		e := script[i]
		i++
		return e
	}
	type result struct {
		token string
		cli   *output.CLIError
	}
	done := make(chan result, 1)
	go func() {
		tok, cli := Poll(fc, interval, expiresIn, exchange)
		done <- result{tok, cli}
	}()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case r := <-done:
			return r.token, r.cli, fc
		case <-deadline:
			t.Fatal("poll did not finish")
		default:
			if fc.WaitForSleeper(10 * time.Millisecond) {
				fc.Advance(10 * time.Second)
			}
		}
	}
}

func TestPollSucceedsAfterPending(t *testing.T) {
	tok, cli, _ := runPoll(t, 5, 300, []Exchange{
		{RFCCode: "authorization_pending"},
		{RFCCode: "authorization_pending"},
		{AccessToken: "sess_tok_win"},
	})
	if cli != nil {
		t.Fatalf("unexpected error: %v", cli)
	}
	if tok != "sess_tok_win" {
		t.Fatalf("token = %q", tok)
	}
}

func TestPollSlowDownBacksOff(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	calls := 0
	var gaps []time.Duration
	last := fc.Now()
	exchange := func() Exchange {
		gaps = append(gaps, fc.Since(last))
		last = fc.Now()
		calls++
		if calls == 1 {
			return Exchange{RFCCode: "slow_down"}
		}
		return Exchange{AccessToken: "tok"}
	}
	done := make(chan struct{})
	go func() {
		_, _ = Poll(fc, 5, 300, exchange)
		close(done)
	}()
	for {
		select {
		case <-done:
			if gaps[0] != 5*time.Second {
				t.Fatalf("first gap = %v, want 5s", gaps[0])
			}
			if gaps[1] != 10*time.Second {
				t.Fatalf("post-slow_down gap = %v, want 10s", gaps[1])
			}
			return
		default:
			if fc.WaitForSleeper(10 * time.Millisecond) {
				fc.Advance(1 * time.Second)
			}
		}
	}
}

func TestPollDeniedStopsWithAuthError(t *testing.T) {
	_, cli, _ := runPoll(t, 5, 300, []Exchange{{RFCCode: "access_denied"}})
	if cli == nil || cli.Code != output.ErrAuth {
		t.Fatalf("want auth error, got %v", cli)
	}
	if cli.Message != "Login denied." {
		t.Fatalf("message = %q", cli.Message)
	}
}

func TestPollExpiredTokenStops(t *testing.T) {
	_, cli, _ := runPoll(t, 5, 300, []Exchange{{RFCCode: "expired_token"}})
	if cli == nil || cli.Code != output.ErrAuth {
		t.Fatalf("want auth error, got %v", cli)
	}
}

func TestPollDeadlineExpires(t *testing.T) {
	script := make([]Exchange, 4)
	for i := range script {
		script[i] = Exchange{RFCCode: "authorization_pending"}
	}
	_, cli, _ := runPoll(t, 5, 12, script)
	if cli == nil || cli.Code != output.ErrAuth {
		t.Fatalf("want auth expiry error, got %v", cli)
	}
}

func TestPollIntervalFloor(t *testing.T) {
	fc := clock.NewFake(time.Unix(1_700_000_000, 0))
	start := fc.Now()
	var firstGap time.Duration
	done := make(chan struct{})
	go func() {
		_, _ = Poll(fc, 0, 60, func() Exchange {
			firstGap = fc.Since(start)
			return Exchange{AccessToken: "tok"}
		})
		close(done)
	}()
	for {
		select {
		case <-done:
			if firstGap != 5*time.Second {
				t.Fatalf("gap with interval=0 is %v, want 5s floor", firstGap)
			}
			return
		default:
			if fc.WaitForSleeper(10 * time.Millisecond) {
				fc.Advance(1 * time.Second)
			}
		}
	}
}
