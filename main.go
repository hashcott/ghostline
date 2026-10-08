package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/headless"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/shell"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Headless modes must branch before application.New: single-instance
	// handling lives inside New and would exit or steal the lock.
	mode, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	p, err := platform.New(exe)
	if err != nil {
		if mode.Kind == cli.KindUI || mode.Kind == cli.KindAutostart {
			shell.Fatal(err)
		}
		fmt.Fprintln(os.Stderr, "Ghostline:", err)
		os.Exit(1)
	}
	switch mode.Kind {
	case cli.KindWatchdog, cli.KindRestore, cli.KindRemoveCerts, cli.KindExport:
		os.Exit(headless.Run(mode, p))
	}
	if err := shell.Run(shell.Options{Mode: mode, Assets: assets, Executable: exe, Platform: p}); err != nil {
		os.Exit(1)
	}
}
