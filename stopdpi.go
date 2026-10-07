package main

import (
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/shell"
)

// stopDPI stops what a dead Ghostline's DPI engine left behind (on Windows,
// the WinDivert services). The engine process itself is already gone: it
// lived in a kill-on-close job.
func stopDPI(p platform.Deps) func() error {
	return shell.NewDPIManager(p.Paths, p, nil).Stop
}
