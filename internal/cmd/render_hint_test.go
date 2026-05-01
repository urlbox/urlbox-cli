package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNetworkHint_Timeout_RenderCall_ListsThreeRecoveryPaths(t *testing.T) {
	h := networkHint(context.DeadlineExceeded, true, 60*time.Second)
	// Three concrete recovery paths:
	//   (a) retry the same command
	//   (b) raise --timeout
	//   (c) switch to --async + --webhook-url
	for _, want := range []string{"retry the same command", "--timeout", "--async"} {
		if !strings.Contains(h, want) {
			t.Errorf("hint=%q, missing recovery path %q", h, want)
		}
	}
	if !strings.Contains(h, "60s") && !strings.Contains(h, "1m0s") {
		t.Errorf("hint should name the actual timeout (60s/1m0s); got %q", h)
	}
	if strings.Contains(h, "internet connection") {
		t.Errorf("render-timeout hint should not blame the connection; got %q", h)
	}
}

func TestNetworkHint_Timeout_NonRenderCall_GenericHint(t *testing.T) {
	h := networkHint(context.DeadlineExceeded, false, 30*time.Second)
	if !strings.Contains(h, "doctor") {
		t.Errorf("non-render-timeout hint should mention urlbox doctor; got %q", h)
	}
	if strings.Contains(h, "--timeout") {
		t.Errorf("non-render hint should not advertise --timeout (render-only flag); got %q", h)
	}
}

func TestNetworkHint_GenericNetworkError_FallsBackToConnectionHint(t *testing.T) {
	h := networkHint(errors.New("dial tcp: connection refused"), true, 60*time.Second)
	if !strings.Contains(h, "internet connection") && !strings.Contains(h, "API host") {
		t.Errorf("generic-network hint missing connection guidance; got %q", h)
	}
	if strings.Contains(h, "--timeout") {
		t.Errorf("non-timeout hint should not suggest --timeout; got %q", h)
	}
}

func TestNetworkHint_WrappedDeadlineExceeded_StillClassified(t *testing.T) {
	wrapped := errors.New("Get \"https://api.urlbox.com/v1/screenshot\": " + context.DeadlineExceeded.Error())
	// errors.Is may or may not catch this depending on wrapping;
	// the hint logic uses errors.Is + a string-contains fallback.
	h := networkHint(wrapped, true, 60*time.Second)
	if !strings.Contains(h, "--timeout") {
		t.Errorf("wrapped DeadlineExceeded should still classify as timeout; got %q", h)
	}
}
