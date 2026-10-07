package sysdns

import (
	"errors"
	"fmt"

	"github.com/hashcott/ghostline/internal/netwatch"
)

// resolvedDropInDir holds systemd-resolved's runtime drop-ins (/run: gone
// at reboot).
const resolvedDropInDir = "/run/systemd/resolved.conf.d"

// detect picks the first usable backend: NetworkManager (running, in
// charge of DNS, and its result reaching the system), systemd-resolved
// (running), then fallback (resolv.conf). nm or resolved may be nil when
// D-Bus is unavailable.
//
// Recovery goes through the backend a snapshot names, which may not be the
// one picked now (NM restarting, or switched to dns=none since): the
// returned backend routes StillOurs and Restore by Snapshot.Backend.
func detect(nm nmAPI, resolved resolvedAPI, units unitAPI, fallback Backend, flush func() error, watch netwatch.WatchFunc) Backend {
	resolvedUp := resolved != nil && resolved.Running()
	r := &router{byName: map[string]Backend{fallback.Name(): fallback}}
	var nmb, rb Backend
	if nm != nil {
		chain := "NetworkManager"
		if resolvedUp {
			chain += " → systemd-resolved"
		}
		nmb = newNM(nm, flush, watch, chain)
		r.byName[nmb.Name()] = nmb
	}
	if resolved != nil {
		rb = newResolved(resolved, units, resolvedDropInDir, watch)
		r.byName[rb.Name()] = rb
	}
	switch {
	case nmb != nil && nm.Running() && nmInCharge(nm):
		r.Backend = nmb
	case resolvedUp:
		r.Backend = rb
	default:
		r.Backend = fallback
	}
	return r
}

// nmInCharge reports whether NM's global DNS reaches the system: NM
// handles DNS, and either hands it to resolved or writes resolv.conf.
func nmInCharge(nm nmAPI) bool {
	mode, err := nm.DNSMode()
	if err != nil || mode == "none" {
		return false
	}
	if mode == "systemd-resolved" {
		return true
	}
	rc, err := nm.RcManager()
	return err == nil && rc != "unmanaged"
}

// router is the detected backend, with recovery sent to the backend each
// snapshot was taken with.
type router struct {
	Backend
	byName map[string]Backend
}

func (r *router) StillOurs(s Snapshot) Snapshot {
	if s.Empty() {
		return s
	}
	b, ok := r.byName[s.Backend]
	if !ok {
		return s // cannot check: keep it, Restore says why
	}
	return b.StillOurs(s)
}

func (r *router) Restore(s Snapshot) []RestoreError {
	if s.Empty() {
		return nil
	}
	b, ok := r.byName[s.Backend]
	if !ok {
		return []RestoreError{{Target: s.Backend, Err: fmt.Errorf("sysdns: %q is not available on this system", s.Backend)}}
	}
	return b.Restore(s)
}

// RestoreDefault clears whatever is plainly Ghostline's in every backend.
func (r *router) RestoreDefault() error {
	var errs []error
	for _, b := range r.byName {
		errs = append(errs, b.RestoreDefault())
	}
	return errors.Join(errs...)
}
