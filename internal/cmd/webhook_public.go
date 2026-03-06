//go:build !internal

package cmd

import (
	"fmt"
	"os"
)

func runWebhook(args []string) int {
	fmt.Fprintf(os.Stderr, "unknown command %q\n\n", "webhook")
	printUsage()
	return 1
}
