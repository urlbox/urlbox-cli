//go:build internal

package cmd

import (
	"strings"
	"testing"
)

func TestInternalBuildEnablesInternalFeatures(t *testing.T) {
	if !internalFeatures {
		t.Fatal("expected internal features to be enabled in internal build")
	}

	output, _ := captureStdoutAndStderr(t, func() int {
		printUsage()
		return 0
	})
	if !strings.Contains(output, "webhook") {
		t.Fatalf("expected internal usage to show webhook, got:\n%s", output)
	}
}
