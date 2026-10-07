package shell

import (
	"log/slog"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/hashcott/ghostline/internal/watchdog"
)

// RecoveryDeps wires recovery to this OS. The GUI's startup restore and
// the headless --restore, --watchdog and --remove-certs modes all use it,
// so none of them can miss a cleanup step.
func RecoveryDeps(p platform.Deps, states *store.StateStore, stopDPI func() error, log *slog.Logger) watchdog.Deps {
	return watchdog.Deps{
		States:          states,
		DNS:             sysdns.NewManager(p.DNS, time.Sleep),
		StopDPI:         stopDPI,
		Alive:           p.Procs.Alive,
		Log:             log,
		RestoreSysProxy: sysproxy.Manager{API: p.SysProxy}.RestoreIfOurs,
		DeleteRule:      p.Firewall.DeleteNamed,
		RemoveCert: func(t string) error {
			// state.json is user-writable: remove only Fake SNI roots.
			return certstore.RemoveIfPrefix(p.Certs, t, certs.SessionPrefix)
		},
		SweepSession: func(keep []string) error {
			_, err := certstore.Sweep(p.Certs, certs.SessionPrefix, keep)
			return err
		},
	}
}
