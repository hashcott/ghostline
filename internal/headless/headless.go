// Package headless runs Ghostline's modes without a window: --restore,
// --watchdog, --remove-certs and --export. Both the Windows exe and the
// Linux daemon use it.
package headless

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/core"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/watchdog"
)

// Run handles --watchdog, --restore, --remove-certs and --export and
// returns the exit code. It never touches Wails.
func Run(mode cli.Mode, p platform.Deps) int {
	paths := p.Paths
	logger := slog.Default()
	if w, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3); err == nil {
		defer w.Close()
		logger = slog.New(slog.NewTextHandler(w, nil)).With("mode", mode.Kind.String())
		// Packages that log through slog.Default (store, backup, …) land in
		// the file too, while Run runs.
		prev := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(prev)
	}
	logger.Info("headless start", "version", brand.Version, "portable", paths.Portable, "admin", p.Procs.IsAdmin())
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
		logger.Info("watchdog: watching", "parent", mode.ParentPID)
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
		// systemd's journal shows stderr (Windows has no console: a no-op).
		fmt.Fprintln(os.Stderr, "Ghostline: recovery failed:", err)
		return 1
	}
	return 0
}

// stopDPI stops what a dead Ghostline's DPI engine left behind (on Windows,
// the WinDivert services). The engine process itself is already gone: it
// lived in a kill-on-close job.
func stopDPI(p platform.Deps) func() error {
	return core.NewDPIManager(p.Paths, p, nil).Stop
}
