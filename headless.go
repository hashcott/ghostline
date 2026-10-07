package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/hashcott/ghostline/internal/winutil"
)

// runHeadless handles --watchdog, --restore, --remove-certs and --export. It never touches Wails.
func runHeadless(mode cli.Mode) int {
	exe, err := os.Executable()
	if err != nil {
		return 1
	}
	paths := store.WithMachineDir(store.ResolvePaths(exe, os.Getenv("APPDATA")), filepath.Join(os.Getenv("ProgramData"), brand.AppName))
	logger := slog.Default()
	if w, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3); err == nil {
		defer w.Close()
		logger = slog.New(slog.NewTextHandler(w, nil)).With("mode", modeName(mode.Kind))
	}
	if mode.Kind == cli.KindExport {
		// Read-only: no state lock, no recovery. A GUI exe has no console of
		// its own: borrow the caller's so the result can be read.
		winutil.AttachParentConsole()
		if err := app.ExportTo(paths, mode.ExportPath, brand.Version); err != nil {
			logger.Error("export failed", "err", err)
			fmt.Fprintln(os.Stderr, "Ghostline: export failed:", err)
			return 1
		}
		_, _ = fmt.Fprintln(os.Stdout, "Ghostline: settings exported to", mode.ExportPath)
		return 0
	}
	lock, err := winutil.NewNamedMutex(brand.StateMutex)
	if err != nil {
		logger.Error("state mutex", "err", err)
		return 1
	}
	roots := certstore.NewWindows(certstore.LocalMachine)
	d := watchdog.Deps{
		States:  store.NewStateStore(paths.State, lock),
		DNS:     sysdns.NewManager(sysdns.NewWindowsAPI(), time.Sleep),
		StopDPI: stopDPI(paths),
		Alive:   winutil.ProcessAlive,
		Log:     logger,

		RestoreSysProxy: sysproxy.Manager{API: sysproxy.NewWindowsAPI()}.RestoreIfOurs,
		DeleteRule:      firewall.NewNetsh(exe).DeleteNamed,
		RemoveCert: func(t string) error {
			// state.json is user-writable: remove only Fake SNI roots.
			return certstore.RemoveIfPrefix(roots, t, certs.SessionPrefix)
		},
		SweepSession: sweepSession(roots),
	}
	switch mode.Kind {
	case cli.KindWatchdog:
		err = watchdog.RunWatchdog(mode.ParentPID, mode.ParentStart, winutil.WaitForExit, d)
	case cli.KindRemoveCerts:
		// The uninstaller: undo whatever a run left, then remove every
		// Ghostline root and the LAN CA files.
		err = errors.Join(watchdog.RunRestore(d), watchdog.RemoveAllCerts(roots, paths.LANCACert, paths.LANCAKey))
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

// sweepSession removes Fake SNI roots not in keep.
func sweepSession(s certstore.Store) func(keep []string) error {
	return func(keep []string) error {
		_, err := certstore.Sweep(s, certs.SessionPrefix, keep)
		return err
	}
}
