package sysdns

import (
	"errors"
	"fmt"
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

func (b *adapterBackend) Apply(s Snapshot, v6 bool) error { return b.m.ApplyLoopback(s.Windows, v6) }

// Reconcile records and redirects adapters that appeared since s was
// taken. Each one is in the returned snapshot before it is changed; one
// that cannot be redirected stays recorded and is reported in the error.
func (b *adapterBackend) Reconcile(s Snapshot, sel Selection, v6 bool) (Snapshot, []Change, error) {
	ads, err := b.m.Select(sel.Mode, sel.IDs)
	if err != nil {
		return s, nil, err
	}
	known := map[string]bool{}
	for _, a := range s.Windows {
		known[a.GUID] = true
	}
	next := s
	next.Windows = append([]model.AdapterSnapshot(nil), s.Windows...)
	var changes []Change
	var errs []error
	for _, a := range ads {
		if known[a.GUID] {
			continue
		}
		snaps, err := b.m.Snapshot([]Adapter{a})
		if err != nil || len(snaps) == 0 {
			continue
		}
		next.Windows = append(next.Windows, snaps...)
		if err := b.m.ApplyLoopback(snaps, v6); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", a.Alias, err))
			continue
		}
		changes = append(changes, Change{Target: a.Alias, Added: true})
	}
	if len(changes) > 0 {
		_ = b.m.Flush()
	}
	return next, changes, errors.Join(errs...)
}

// StillOurs keeps the adapters whose DNS still points at loopback: one the
// user re-configured after a crash keeps their settings. If the current
// DNS cannot be read, everything is kept.
func (b *adapterBackend) StillOurs(s Snapshot) Snapshot {
	ads, err := b.m.LoopbackAdapters()
	if err != nil {
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

func (b *adapterBackend) Info() Info {
	ads, _ := b.api.Adapters()
	names := []string{}
	for _, a := range ads {
		names = append(names, a.Alias)
	}
	return Info{Backend: b.Name(), Chain: "Windows", Interfaces: names, AdapterPick: true, Adapters: ads}
}
