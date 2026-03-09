package main

import (
	"fmt"
	"os"

	"github.com/allyourbase/ayb/internal/cli"
	"github.com/allyourbase/ayb/internal/cli/ui"
)

// Set by goreleaser at build time.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cli.SetVersion(version, commit, date)
	if err := cli.Execute(); err != nil {
		fmt.Fprint(os.Stderr, ui.FormatError(err.Error()))
		os.Exit(1)
	}
}
