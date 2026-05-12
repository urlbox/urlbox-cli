// internal/cmd/doctor_internal_test.go
package cmd

import (
	"testing"
	"time"
)

// TestDoctor_HttpTimeout_AtLeast10s pins Round 5 CI-1: the per-check
// HTTP timeout used to be 5s, which false-failed on cold-container
// invocations where DNS+TCP+TLS to api.urlbox.com exceeded the budget
// even though the warm-cache request returned in ~350ms. 10s is the
// floor — any value below this regresses CI health-check usage.
func TestDoctor_HttpTimeout_AtLeast10s(t *testing.T) {
	const minTimeout = 10 * time.Second
	if httpTimeout < minTimeout {
		t.Errorf("httpTimeout = %v, want >= %v for cold-start DNS resilience (Round 5 CI-1)", httpTimeout, minTimeout)
	}
}
