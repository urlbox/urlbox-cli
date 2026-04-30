package api_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/urlbox/urlbox-cli/internal/api"
)

// recordingSleeper captures every Sleep duration without actually sleeping.
type recordingSleeper struct {
	durations []time.Duration
}

func (r *recordingSleeper) Sleep(d time.Duration) {
	r.durations = append(r.durations, d)
}

func TestRetryDo_Success_FirstTry_NoSleep(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = s.Sleep

	var calls int32
	do := func() (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	}

	resp, err := api.RetryDo(context.Background(), cfg, do)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status=%d", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("calls=%d, want 1 (no retries on success)", calls)
	}
	if len(s.durations) != 0 {
		t.Errorf("slept %v, want zero sleeps on first-try success", s.durations)
	}
}

func TestRetryDo_503_503_200_Sleeps_1s_2s(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = s.Sleep
	cfg.Jitter = 0 // disable jitter to pin durations

	sequence := []int{503, 503, 200}
	var i int32
	do := func() (*http.Response, error) {
		idx := atomic.AddInt32(&i, 1) - 1
		return &http.Response{StatusCode: sequence[idx], Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	resp, err := api.RetryDo(context.Background(), cfg, do)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("final status=%d, want 200", resp.StatusCode)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second}
	if len(s.durations) != len(want) {
		t.Fatalf("slept %v, want %v", s.durations, want)
	}
	for k, d := range want {
		if s.durations[k] != d {
			t.Errorf("sleep[%d]=%v, want %v", k, s.durations[k], d)
		}
	}
}

func TestRetryDo_429_RespectsRetryAfter(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = s.Sleep
	cfg.Jitter = 0

	var i int32
	do := func() (*http.Response, error) {
		idx := atomic.AddInt32(&i, 1) - 1
		if idx == 0 {
			return &http.Response{
				StatusCode: 429,
				Header:     http.Header{"Retry-After": []string{"5"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	resp, err := api.RetryDo(context.Background(), cfg, do)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("final status=%d", resp.StatusCode)
	}
	if len(s.durations) != 1 {
		t.Fatalf("slept %v, want one sleep", s.durations)
	}
	if s.durations[0] != 5*time.Second {
		t.Errorf("sleep[0]=%v, want 5s (Retry-After honored over backoff)", s.durations[0])
	}
}

func TestRetryDo_NoRetryConfig_DoesNotRetry(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.NoRetryConfig()
	cfg.Sleep = s.Sleep

	var calls int32
	do := func() (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
	}

	resp, err := api.RetryDo(context.Background(), cfg, do)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 503 {
		t.Errorf("status=%d", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("calls=%d, want 1 (no-retry mode)", calls)
	}
	if len(s.durations) != 0 {
		t.Errorf("slept %v, want none in no-retry mode", s.durations)
	}
}

func TestRetryDo_BudgetExhausted_ReturnsLastResponse(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = s.Sleep
	cfg.Jitter = 0

	var calls int32
	do := func() (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("server boom"))}, nil
	}

	resp, err := api.RetryDo(context.Background(), cfg, do)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 503 {
		t.Errorf("final status=%d, want 503 (budget exhausted)", resp.StatusCode)
	}
	if calls != 4 {
		t.Errorf("calls=%d, want 4 (1 initial + 3 retries)", calls)
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}
	if len(s.durations) != len(want) {
		t.Fatalf("slept %v, want %v", s.durations, want)
	}
	for k, d := range want {
		if s.durations[k] != d {
			t.Errorf("sleep[%d]=%v, want %v", k, s.durations[k], d)
		}
	}
}

func TestRetryDo_NetworkError_RetriesUntilBudgetExhausted(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = s.Sleep
	cfg.Jitter = 0

	var calls int32
	netErr := errors.New("dial tcp: i/o timeout")
	do := func() (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return nil, netErr
	}

	resp, err := api.RetryDo(context.Background(), cfg, do)
	if !errors.Is(err, netErr) {
		t.Errorf("err=%v, want %v", err, netErr)
	}
	if resp != nil {
		_ = resp.Body.Close()
		t.Errorf("resp=%v, want nil", resp)
	}
	if calls != 4 {
		t.Errorf("calls=%d, want 4", calls)
	}
}

func TestRetryDo_ContextCancelled_MidSleep_Aborts(t *testing.T) {
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = time.Sleep
	cfg.BaseDelay = 50 * time.Millisecond
	cfg.Jitter = 0

	ctx, cancel := context.WithCancel(context.Background())
	do := func() (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	resp, err := api.RetryDo(ctx, cfg, do)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want context.Canceled", err)
	}
}

func TestRetryDo_4xxNot429_ReturnsImmediately(t *testing.T) {
	s := &recordingSleeper{}
	cfg := api.DefaultRetryConfig()
	cfg.Sleep = s.Sleep

	var calls int32
	do := func() (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
	}
	resp, err := api.RetryDo(context.Background(), cfg, do)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 404 {
		t.Errorf("status=%d", resp.StatusCode)
	}
	if calls != 1 {
		t.Errorf("calls=%d, want 1 (4xx is not retryable except 429)", calls)
	}
}
