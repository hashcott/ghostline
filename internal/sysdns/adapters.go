package sysdns

import (
	"errors"
	"log/slog"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// adapterBackend changes DNS per network adapter (Windows).
type adapterBackend struct {
	api   API
	m     *Manager
	watch func(onChange func()) (func(), error)
}

// NewAdapterBackend changes adapters through api; watch reports adapter
// changes.
func NewAdapterBackend(api API, watch func(onChange func()) (func(), error)) Backend {
	return &adapterBackend{api: api, m: NewManager(api, time.Sleep), watch: watch}
}

func (b *adapterBackend) Name() string { return "windows" }

func (b *adapterBackend) Snapshot(sel Selection) (Snapshot, error) {
	ads, err := b.m.Select(sel.Mode, sel.IDs)
	if err != nil {
		return Snapshot{}, err
	}
	if len(ads) == 0 {
		return Snapshot{}, errors.New("no connected adapters")
	}
	snaps, err := b.m.Snapshot(ads)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Backend: b.Name(), Windows: snaps}, nil
}

// Report lists the adapters that are up with their DNS servers, so a failed
// leak check can name the ones that bypass Ghostline.
func (b *adapterBackend) Report() ([]AdapterDNS, error) { return b.m.Report() }

func (b *adapterBackend) Apply(s Snapshot, v6 bool) error { return b.m.ApplyLoopback(s.Windows, v6) }

// ApplyLoopbackNetsh sets loopback again through netsh (see
// Manager.ApplyLoopbackNetsh).
func (b *adapterBackend) ApplyLoopbackNetsh(s Snapshot, v6 bool) error {
	return b.m.ApplyLoopbackNetsh(s.Windows, v6)
}

// Reconcile records adapters that appeared since s was taken; toApply
// holds only them, so adapters already handled are not touched again.
func (b *adapterBackend) Reconcile(s Snapshot, sel Selection) (Snapshot, Snapshot, []Change, error) {
	ads, err := b.m.Select(sel.Mode, sel.IDs)
	if err != nil {
		return s, Snapshot{}, nil, err
	}
	known := map[string]bool{}
	for _, a := range s.Windows {
		known[a.GUID] = true
	}
	next := s
	next.Windows = append([]model.AdapterSnapshot(nil), s.Windows...)
	toApply := Snapshot{Backend: s.Backend}
	var changes []Change
	for _, a := range ads {
		if known[a.GUID] {
			continue
		}
		snaps, err := b.m.Snapshot([]Adapter{a})
		if err != nil || len(snaps) == 0 {
			continue
		}
		next.Windows = append(next.Windows, snaps...)
		toApply.Windows = append(toApply.Windows, snaps...)
		changes = append(changes, Change{Target: a.Alias, Added: true})
	}
	return next, toApply, changes, nil
}

// StillOurs keeps the adapters whose DNS still points at loopback: one the
// user re-configured after a crash keeps their settings. If the current
// DNS cannot be read, everything is kept.
func (b *adapterBackend) StillOurs(s Snapshot) Snapshot {
	ads, err := b.m.LoopbackAdapters()
	if err != nil {
		slog.Warn("sysdns: read current adapter DNS failed; restoring every snapshot", "err", err)
		return s
	}
	on := make(map[string]bool, len(ads))
	for _, a := range ads {
		on[a.GUID] = true
	}
	out := Snapshot{Backend: s.Backend}
	for _, a := range s.Windows {
		if on[a.GUID] {
			out.Windows = append(out.Windows, a)
		}
	}
	return out
}

func (b *adapterBackend) Restore(s Snapshot) []RestoreError { return b.m.Restore(s.Windows) }

// RestoreDefault resets every adapter still pointing at loopback to DHCP.
func (b *adapterBackend) RestoreDefault() error {
	ads, err := b.m.LoopbackAdapters()
	if err != nil {
		return err
	}
	var snaps []model.AdapterSnapshot
	for _, a := range ads {
		snaps = append(snaps, model.AdapterSnapshot{GUID: a.GUID, LUID: a.LUID, IfIndex: a.IfIndex, Alias: a.Alias,
			IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}, IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}})
	}
	var errs []error
	for _, e := range b.m.Restore(snaps) {
		errs = append(errs, e)
	}
	return errors.Join(errs...)
}

func (b *adapterBackend) Flush() error { return b.m.Flush() }

func (b *adapterBackend) Watch(onChange func()) (func(), error) { return b.watch(onChange) }

func (b *adapterBackend) Adapters() ([]Adapter, error) { return b.api.Adapters() }

func (b *adapterBackend) Info() Info {
	ads, _ := b.api.Adapters()
	names := []string{}
	for _, a := range ads {
		names = append(names, a.Alias)
	}
	return Info{Backend: b.Name(), Chain: "Windows", Interfaces: names, AdapterPick: true, Adapters: ads}
}
