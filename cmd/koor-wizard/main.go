package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DavidRHerbert/koor/internal/wizard"
)

func main() {
	accessible := flag.Bool("accessible", false, "run in accessible mode (no TUI chrome)")
	flag.Parse()

	if err := wizard.Run(wizard.Options{Accessible: *accessible}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
