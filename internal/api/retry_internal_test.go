package api

import (
	"net/http"
	"testing"
	"time"
)

// White-box tests for unexported retry helpers. Locks down boundary
// behavior that the integration tests in retry_test.go don't pin directly.

func TestParseRetryAfter_Seconds(t *testing.T) {
	now := time.Date(2026, 4, 30, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		header    string
		wantDur   time.Duration
		wantValid bool
	}{
		{"missing", "", 0, false},
		{"zero", "0", 0, true},
		{"positive", "5", 5 * time.Second, true},
		{"large", "300", 300 * time.Second, true},
		{"whitespace", "  3  ", 3 * time.Second, true},
		{"negative", "-1", 0, false},
		{"non-numeric garbage", "soon", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.header != "" {
				h.Set("Retry-After", tc.header)
			}
			d, ok := parseRetryAfter(h, now)
			if d != tc.wantDur || ok != tc.wantValid {
				t.Errorf("parseRetryAfter(%q) = (%v, %v), want (%v, %v)", tc.header, d, ok, tc.wantDur, tc.wantValid)
			}
		})
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		date      string
		wantDur   time.Duration
		wantValid bool
	}{
		{"future", "Thu, 30 Apr 2026 12:00:30 GMT", 30 * time.Second, true},
		{"past", "Thu, 30 Apr 2026 11:59:30 GMT", 0, false},
		{"now", "Thu, 30 Apr 2026 12:00:00 GMT", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{"Retry-After": []string{tc.date}}
			d, ok := parseRetryAfter(h, now)
			if ok != tc.wantValid {
				t.Errorf("parseRetryAfter(%q) ok = %v, want %v", tc.date, ok, tc.wantValid)
			}
			if ok && d != tc.wantDur {
				t.Errorf("parseRetryAfter(%q) dur = %v, want %v", tc.date, d, tc.wantDur)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	cases := []struct {
		status int
		want   bool
	}{
		{200, false},
		{301, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{409, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
		{504, true},
	}
	for _, tc := range cases {
		if got := isRetryable(tc.status); got != tc.want {
			t.Errorf("isRetryable(%d) = %v, want %v", tc.status, got, tc.want)
		}
	}
}
