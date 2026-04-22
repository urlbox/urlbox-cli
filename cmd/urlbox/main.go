package main

import (
	"os"

	"github.com/urlbox/cli/internal/cmd"
)

func main() {
	code := cmd.Execute(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}
