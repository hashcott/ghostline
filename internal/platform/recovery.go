package platform

import (
	"log/slog"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/watchdog"
)

// Recovery wires recovery to this OS. The GUI's startup restore and the
// headless --restore, --watchdog and --remove-certs modes all use it (and
// the Linux daemon will), so none of them can miss a cleanup step.
func (d Deps) Recovery(states *store.StateStore, stopDPI func() error, log *slog.Logger) watchdog.Deps {
	return watchdog.Deps{
		States:          states,
		DNS:             d.DNS,
		StopDPI:         stopDPI,
		Alive:           d.Procs.Alive,
		Log:             log,
		RestoreSysProxy: d.SysProxy.RestoreIfOurs,
		DeleteRule:      d.Firewall.DeleteNamed,
		RemoveCert: func(t string) error {
			// state.json is user-writable: remove only Fake SNI roots.
			return certstore.RemoveIfPrefix(d.Certs, t, certs.SessionPrefix)
		},
		SweepSession: func(keep []string) error {
			_, err := certstore.Sweep(d.Certs, certs.SessionPrefix, keep)
			return err
		},
	}
}
