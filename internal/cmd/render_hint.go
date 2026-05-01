package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// networkHint classifies a network-class error and returns the most useful
// remediation hint. For render calls on a deadline timeout, it names three
// concrete recovery paths (retry, raise --timeout, switch to --async). For
// non-render calls or non-timeout errors, it falls back to generic guidance.
//
// timeout is the actual deadline the call ran under; it's echoed in the
// hint so agents see exactly how long they waited.
func networkHint(err error, isRenderCall bool, timeout time.Duration) string {
	if err == nil {
		return ""
	}
	if isTimeoutErr(err) {
		if isRenderCall {
			return fmt.Sprintf(
				"Render timed out after %s. Try one of: (a) retry the same command — slow renders sometimes succeed second try; (b) --timeout %s for a longer single attempt; (c) --async --webhook-url <url> for very heavy pages.",
				timeout, suggestLongerTimeout(timeout),
			)
		}
		return "Request timed out. Run `urlbox doctor` to check API reachability."
	}
	return "Check your internet connection and the API host (URLBOX_API_HOST). Run `urlbox doctor`."
}

// suggestLongerTimeout returns a "try N" value 3× the current timeout,
// floored at 30s and capped at 10m. The floor avoids the degenerate
// "try --timeout 0s" hint when the test or caller passes a sub-second
// timeout (Round(time.Second) on a 300ms value yields 0s).
func suggestLongerTimeout(current time.Duration) time.Duration {
	suggested := current * 3
	if suggested > 10*time.Minute {
		suggested = 10 * time.Minute
	}
	if suggested < 30*time.Second {
		suggested = 30 * time.Second
	}
	return suggested.Round(time.Second)
}

// isTimeoutErr returns true for context.DeadlineExceeded — including when
// wrapped by net/http or fmt.Errorf(%w). Falls back to a string match
// because not all upstream wrappers honor errors.Is.
func isTimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), context.DeadlineExceeded.Error())
}
