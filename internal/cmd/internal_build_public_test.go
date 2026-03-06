//go:build !internal

package cmd

import (
	"strings"
	"testing"
)

func TestPublicBuildDisablesInternalFeatures(t *testing.T) {
	if internalFeatures {
		t.Fatal("expected internal features to be disabled in public build")
	}

	output, _ := captureStdoutAndStderr(t, func() int {
		printUsage()
		return 0
	})
	if strings.Contains(output, "webhook") {
		t.Fatalf("expected public usage to hide webhook, got:\n%s", output)
	}
}
