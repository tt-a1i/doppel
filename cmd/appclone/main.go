package main

import (
	"fmt"
	"os"

	"github.com/tt-a1i/appclone/internal/cli"
	"github.com/tt-a1i/appclone/internal/tui"
)

func main() {
	if len(os.Args) > 1 {
		if err := cli.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := tui.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
