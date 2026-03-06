package main

import (
	"os"

	"github.com/urlbox/cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:]))
}
