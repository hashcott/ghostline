package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/watchdog"
)

// runHeadless handles --watchdog, --restore, --remove-certs and --export. It never touches Wails.
func runHeadless(mode cli.Mode, p platform.Deps) int {
	paths := p.Paths
	logger := slog.Default()
	if w, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3); err == nil {
		defer w.Close()
		logger = slog.New(slog.NewTextHandler(w, nil)).With("mode", modeName(mode.Kind))
	}
	if mode.Kind == cli.KindExport {
		// Read-only: no state lock, no recovery. A GUI exe has no console of
		// its own: borrow the caller's so the result can be read.
		p.AttachConsole()
		if err := app.ExportTo(paths, mode.ExportPath, brand.Version); err != nil {
			logger.Error("export failed", "err", err)
			fmt.Fprintln(os.Stderr, "Ghostline: export failed:", err)
			return 1
		}
		_, _ = fmt.Fprintln(os.Stdout, "Ghostline: settings exported to", mode.ExportPath)
		return 0
	}
	d := p.Recovery(store.NewStateStore(paths.State, p.Lock), stopDPI(p), logger)
	var err error
	switch mode.Kind {
	case cli.KindWatchdog:
		err = watchdog.RunWatchdog(mode.ParentPID, mode.ParentStart, p.Procs.WaitForExit, d)
	case cli.KindRemoveCerts:
		// The uninstaller: undo whatever a run left, then remove every
		// Ghostline root and the LAN CA files.
		err = errors.Join(watchdog.RunRestore(d), watchdog.RemoveAllCerts(p.Certs, paths.LANCACert, paths.LANCAKey))
	default:
		err = watchdog.RunRestore(d)
	}
	if err != nil {
		logger.Error("recovery failed", "err", err)
		return 1
	}
	return 0
}

func modeName(k cli.Kind) string {
	switch k {
	case cli.KindWatchdog:
		return "watchdog"
	case cli.KindRestore:
		return "restore"
	case cli.KindRemoveCerts:
		return "remove-certs"
	case cli.KindAutostart:
		return "autostart"
	case cli.KindExport:
		return "export"
	}
	return "ui"
}
