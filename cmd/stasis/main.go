package main

import (
	"os"

	"github.com/sarth-shah20/stasis/cmd"
)

func main() {
	// We delegate all logic to the cmd package.
	// This keeps main clean and allows us to test CLI commands separately.
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}