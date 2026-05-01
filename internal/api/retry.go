package api

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig configures the per-call retry budget. Construct via
// DefaultRetryConfig() or NoRetryConfig() and tweak as needed.
type RetryConfig struct {
	// MaxRetries is the number of *additional* attempts after the first.
	// Default 3 (so total = 4 attempts: initial + 3 retries). Set to 0
	// by NoRetry mode.
	MaxRetries int

	// BaseDelay is the first backoff (1s by default). Doubles each attempt.
	BaseDelay time.Duration

	// MaxDelay caps the backoff. Default 30s.
	MaxDelay time.Duration

	// Jitter as a fraction of the computed delay (0.2 = +/- 20%). Set to 0
	// to disable (useful in tests that need pinned durations).
	Jitter float64

	// Sleep is injectable for tests; default time.Sleep.
	Sleep func(time.Duration)
}

// DefaultRetryConfig returns the spec-default retry policy: 3 retries,
// 1s/2s/4s backoff with +/-20% jitter, respects Retry-After, uses time.Sleep.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  time.Second,
		MaxDelay:   30 * time.Second,
		Jitter:     0.2,
		Sleep:      time.Sleep,
	}
}

// NoRetryConfig returns a config that disables all retries (--no-retry).
func NoRetryConfig() RetryConfig {
	cfg := DefaultRetryConfig()
	cfg.MaxRetries = 0
	return cfg
}

// RetryDo executes do() up to (1 + cfg.MaxRetries) times, sleeping between
// attempts on 429 or 5xx responses (or any do() error). Honors Retry-After
// when present. Returns the last response (success or final failure) once
// the budget is exhausted. If ctx is cancelled mid-sleep, returns ctx.Err().
//
// The caller is responsible for closing resp.Body on the *final* returned
// response. RetryDo drains and closes intermediate response bodies before
// the next attempt.
func RetryDo(ctx context.Context, cfg RetryConfig, do func() (*http.Response, error)) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := do()
		if err == nil && !isRetryable(resp.StatusCode) {
			return resp, nil
		}
		// Timeout means the per-attempt budget is exhausted; retrying will
		// hit it again. Surface the timeout fast so the agent can pick the
		// recovery (retry / --timeout / --async). Net/http wraps the
		// context error in url.Error — errors.Is unwraps it.
		if err != nil && (errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), context.DeadlineExceeded.Error())) {
			return resp, err
		}
		if attempt == cfg.MaxRetries {
			return resp, err
		}

		delay := backoffDelay(attempt, cfg)
		if resp != nil {
			if d, ok := parseRetryAfter(resp.Header, time.Now()); ok {
				delay = d
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}

		// Run the sleep in a goroutine so we can race it against ctx.Done().
		// On ctx cancel, the goroutine survives until cfg.Sleep returns —
		// a bounded leak (capped by MaxDelay = 30s) we accept for now to
		// keep the Sleep injection point a simple `func(time.Duration)`.
		// Cleaner alternative: switch Sleeper to `func(ctx, d) error` and
		// use time.Timer + Stop(). Deferred to v1.0 polish.
		done := make(chan struct{})
		go func() {
			cfg.Sleep(delay)
			close(done)
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

// isRetryable returns true for 429 and 5xx status codes.
func isRetryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// backoffDelay returns the exponential delay for the given 0-based attempt
// index, capped at cfg.MaxDelay and optionally fuzzed by cfg.Jitter.
//
// attempt=0 → BaseDelay; attempt=1 → BaseDelay*2; attempt=2 → BaseDelay*4;
// etc.
func backoffDelay(attempt int, cfg RetryConfig) time.Duration {
	d := cfg.BaseDelay << attempt //nolint:gosec // attempt is bounded by MaxRetries; no overflow risk
	if d > cfg.MaxDelay {
		d = cfg.MaxDelay
	}
	if cfg.Jitter > 0 {
		// rand.Float64() returns [0,1); 2x-1 gives [-1,1).
		// Non-cryptographic: jitter is for spreading retry timing across
		// concurrent clients, not for security.
		mult := 1 + cfg.Jitter*(2*rand.Float64()-1) //nolint:gosec // non-cryptographic jitter
		d = time.Duration(float64(d) * mult)
	}
	return d
}

// parseRetryAfter parses Retry-After: either an integer of seconds, or an
// HTTP-date relative to `now`. Returns 0 + false on missing/invalid/negative.
func parseRetryAfter(h http.Header, now time.Time) (time.Duration, bool) {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := t.Sub(now)
		if d < 0 {
			return 0, false
		}
		return d, true
	}
	return 0, false
}
