// Package watchdog restores DNS when Ghostline's main process dies uncleanly.
// The same logic serves three safety layers: the --watchdog child, startup
// recovery in the app, and the --restore logon task.
package watchdog

import (
	"errors"
	"fmt"
	"github.com/hashcott/ghostline/internal/model"
	"log/slog"
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

// Restorer is the part of a DNS backend recovery needs (sysdns.Backend).
type Restorer interface {
	Restore(sysdns.Snapshot) []sysdns.RestoreError
	// StillOurs keeps what Ghostline's configuration still holds, so a
	// change the user made after a crash is kept.
	StillOurs(sysdns.Snapshot) sysdns.Snapshot
	// RestoreDefault undoes Ghostline's configuration without a snapshot.
	RestoreDefault() error
}

// Deps wires recovery to the system.
type Deps struct {
	States  *store.StateStore
	DNS     Restorer
	StopDPI func() error
	Alive   func(pid uint32, start time.Time) bool
	Log     *slog.Logger
	Sleep   func(time.Duration) // default time.Sleep
	// RestoreSysProxy puts the system proxy back if it is still Ghostline's
	// (sysproxy.Manager.RestoreIfOurs); nil skips it.
	RestoreSysProxy func(ours string, snap model.ProxySnapshot) (bool, error)
	// DeleteRule removes an inbound firewall rule by name (idempotent);
	// nil skips firewall cleanup.
	DeleteRule func(name string) error
	// RemoveCert removes a Fake SNI session CA from the Root store by
	// thumbprint (a missing one is not an error); nil skips it.
	RemoveCert func(thumbprint string) error
	// SweepSession removes every "Ghostline Fake SNI" root not in keep;
	// nil skips it.
	SweepSession func(keep []string) error
}

// AllFirewallRules are the rule names a corrupt state may have left.
var AllFirewallRules = firewall.AllRuleNames

// Outcome says what RestoreIfOrphaned did.
type Outcome int

const (
	NothingToDo Outcome = iota
	Restored
	RestoredFromCorrupt
	OwnerAlive
)

func (d Deps) log() *slog.Logger {
	if d.Log != nil {
		return d.Log
	}
	return slog.Default()
}

// RestoreIfOrphaned restores DNS if state.json says Ghostline changed it and
// the process that did so (pid + start time) is gone. A corrupt state file
// falls back to resetting every adapter still pointing at loopback to DHCP.
func RestoreIfOrphaned(d Deps) (Outcome, error) {
	var out Outcome
	err := d.States.Locked(func() error {
		st, err := d.States.Load()
		if errors.Is(err, store.ErrStateCorrupt) {
			d.log().Warn("state.json corrupt; resetting loopback adapters to DHCP")
			if d.DeleteRule != nil {
				// Idempotent; there is no system proxy snapshot to restore.
				for _, name := range AllFirewallRules {
					if err := d.DeleteRule(name); err != nil {
						d.log().Warn("watchdog: delete firewall rule failed", "err", err, "rule", name)
					}
				}
			}
			// No thumbprints to go by: the sweep removes every session CA.
			defer d.sweep()
			rerr := d.DNS.RestoreDefault()
			if d.StopDPI != nil {
				if err := d.StopDPI(); err != nil {
					d.log().Warn("watchdog: stop DPI engine failed", "err", err)
				}
			}
			out = RestoredFromCorrupt
			if rerr != nil {
				return rerr // leave the corrupt file: the next layer retries
			}
			return d.States.Write(store.CleanState())
		}
		if err != nil {
			return err
		}
		if st.Phase == store.PhaseClean {
			out = NothingToDo
			// A session CA Disconnect could not remove stays listed.
			if err := removeSessionCerts(d, st); err != nil {
				return err
			}
			if st.Certs != nil {
				st.Certs = nil
				if err := d.States.Write(st); err != nil {
					return err
				}
			}
			d.sweep()
			return nil
		}
		if d.Alive(st.PID, st.PIDStartTime) {
			out = OwnerAlive
			return nil
		}
		// Order: session CAs, system proxy, firewall, DNS (spec 2B 6.5).
		cerr := removeSessionCerts(d, st)
		perr := restoreProxy(d, st)
		rerr := joinRestore(d.DNS.Restore(d.DNS.StillOurs(st.DNS)))
		if st.DPI.Running && d.StopDPI != nil {
			if err := d.StopDPI(); err != nil {
				d.log().Warn("watchdog: stop DPI engine failed", "err", err)
			}
		}
		out = Restored
		if err := errors.Join(cerr, perr, rerr); err != nil {
			// Keep the state so a later layer can retry.
			return err
		}
		if err := d.States.Write(store.CleanState()); err != nil {
			return err
		}
		d.sweep()
		return nil
	})
	return out, err
}

// restoreProxy undoes the proxy phase: the system proxy (unless never
// applied or taken over by another app) and the firewall rule.
func restoreProxy(d Deps, st store.State) error {
	var errs []error
	// No Set gate: a crash between Apply and recording Set would otherwise
	// leave the proxy behind; RestoreIfOurs checks the value is ours.
	if sp := st.SysProxy; sp != nil && !sp.TakenOver && sp.Snapshot != nil && d.RestoreSysProxy != nil {
		if _, err := d.RestoreSysProxy(sp.Ours, *sp.Snapshot); err != nil {
			errs = append(errs, fmt.Errorf("watchdog: restore system proxy: %w", err))
		}
	}
	if st.Firewall != nil && d.DeleteRule != nil {
		for _, name := range st.Firewall.Rules {
			if !slices.Contains(AllFirewallRules, name) {
				continue // state.json is user-writable: never delete other rules
			}
			if err := d.DeleteRule(name); err != nil {
				errs = append(errs, fmt.Errorf("watchdog: delete firewall rule %q: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// removeSessionCerts removes the Fake SNI CAs state.json recorded. It runs
// first: a CA left in the Root store is the most dangerous leftover.
func removeSessionCerts(d Deps, st store.State) error {
	if st.Certs == nil || d.RemoveCert == nil {
		return nil
	}
	var errs []error
	for _, t := range st.Certs.Session {
		if err := d.RemoveCert(t); err != nil {
			errs = append(errs, fmt.Errorf("watchdog: remove session CA %s: %w", t, err))
		}
	}
	return errors.Join(errs...)
}

// sweep removes Fake SNI roots no running session owns (none, here: the
// owner is gone or the state is clean).
func (d Deps) sweep() {
	if d.SweepSession == nil {
		return
	}
	if err := d.SweepSession(nil); err != nil {
		d.log().Warn("sweeping Fake SNI certificates failed", "err", err)
	}
}

func joinRestore(errs []sysdns.RestoreError) error {
	var out []error
	for _, e := range errs {
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil
	}
	return fmt.Errorf("watchdog: restore incomplete: %w", errors.Join(out...))
}

// RunWatchdog waits for the parent to exit, then restores if it died without
// cleaning up.
func RunWatchdog(parentPID uint32, parentStart time.Time, wait func(pid uint32) error, d Deps) error {
	sleep := d.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for {
		if err := wait(parentPID); err != nil {
			d.log().Warn("waiting for parent failed", "err", err)
		}
		// A wait that returns while the parent (same pid and start time)
		// is still alive was spurious: keep watching.
		if !d.Alive(parentPID, parentStart) {
			break
		}
		sleep(time.Second)
	}
	_, err := RestoreIfOrphaned(d)
	return err
}

// RunRestore is the --restore mode: one recovery attempt.
func RunRestore(d Deps) error {
	_, err := RestoreIfOrphaned(d)
	return err
}
