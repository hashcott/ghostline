package sysdns

import (
	"errors"
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/netwatch"
)

const nmTarget = "NetworkManager"

// nmBackend points every domain at Ghostline through NetworkManager's
// global DNS configuration. NM then pushes it to systemd-resolved or
// resolv.conf itself, and DHCP renewals do not overwrite it. NM stores the
// value on disk, so recovery must always run (spec Linux §7).
type nmBackend struct {
	api   nmAPI
	flush func() error
	watch netwatch.WatchFunc // network changes; nil for none
	chain string
	sleep func(time.Duration)
}

func newNM(api nmAPI, flush func() error, watch netwatch.WatchFunc, chain string) Backend {
	return &nmBackend{api: api, flush: flush, watch: watch, chain: chain, sleep: time.Sleep}
}

func (b *nmBackend) Name() string { return "networkmanager" }

func (b *nmBackend) Snapshot(Selection) (Snapshot, error) {
	g, err := b.api.GlobalDNS()
	if err != nil {
		return Snapshot{}, err
	}
	servers, _ := b.api.DNSServers()
	return Snapshot{Backend: b.Name(), Linux: &model.LinuxDNS{NMGlobal: &g, Servers: notLoopback(servers)}}, nil
}

func loopbackServers(v6 bool) []string {
	if v6 {
		return []string{"127.0.0.1", "::1"}
	}
	return []string{"127.0.0.1"}
}

// ours reports whether g is Ghostline's: one "*" domain whose servers are
// all loopback.
func ours(g model.NMGlobalDNS) bool {
	if len(g.Domains) != 1 {
		return false
	}
	d, ok := g.Domains["*"]
	if !ok || len(d.Servers) == 0 {
		return false
	}
	for _, s := range d.Servers {
		if s != "127.0.0.1" && s != "::1" {
			return false
		}
	}
	return true
}

func (b *nmBackend) Apply(s Snapshot, v6 bool) error {
	g := model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: loopbackServers(v6)}}}
	if s.Linux != nil && s.Linux.NMGlobal != nil {
		g.Searches = slices.Clone(s.Linux.NMGlobal.Searches)
	}
	if err := b.api.SetGlobalDNS(g); err != nil {
		return err
	}
	// NM applies the change asynchronously: give it up to two seconds.
	for i := 0; i < 20; i++ {
		if servers, err := b.api.DNSServers(); err == nil && len(servers) > 0 && servers[0] == "127.0.0.1" {
			return nil
		}
		b.sleep(100 * time.Millisecond)
	}
	return errors.New("networkmanager: DNS did not change")
}

// Reconcile sets Ghostline's configuration again when something replaced
// it; the snapshot (the value from before Connect) is kept as it is.
func (b *nmBackend) Reconcile(s Snapshot, _ Selection) (Snapshot, Snapshot, []Change, error) {
	cur, err := b.api.GlobalDNS()
	if err != nil {
		return s, Snapshot{}, nil, err
	}
	if ours(cur) {
		return s, Snapshot{}, nil, nil
	}
	return s, s, []Change{{Target: nmTarget}}, nil
}

// StillOurs keeps s only while NM still holds Ghostline's configuration.
// If it cannot be read, everything is restored (the safe side).
func (b *nmBackend) StillOurs(s Snapshot) Snapshot {
	cur, err := b.api.GlobalDNS()
	if err != nil || ours(cur) {
		return s
	}
	return Snapshot{}
}

func (b *nmBackend) Restore(s Snapshot) []RestoreError {
	if s.Linux == nil || s.Linux.NMGlobal == nil {
		return nil
	}
	if err := b.api.SetGlobalDNS(*s.Linux.NMGlobal); err != nil {
		return []RestoreError{{Target: nmTarget, Err: err}}
	}
	return nil
}

// RestoreDefault clears the global configuration only if it is Ghostline's.
func (b *nmBackend) RestoreDefault() error {
	cur, err := b.api.GlobalDNS()
	if err != nil {
		return err
	}
	if !ours(cur) {
		return nil
	}
	return b.api.SetGlobalDNS(model.NMGlobalDNS{})
}

func (b *nmBackend) Flush() error { return b.flush() }

func (b *nmBackend) Watch(onChange func()) (func(), error) {
	if b.watch == nil {
		return b.api.Watch(onChange)
	}
	return netwatch.Combine(b.api.Watch, b.watch)(onChange)
}

func (b *nmBackend) Info() Info {
	return Info{Backend: b.Name(), Chain: b.chain, Interfaces: []string{}}
}

// notLoopback drops loopback servers (Ghostline's own, or a local stub).
func notLoopback(servers []string) []string {
	out := []string{}
	for _, s := range servers {
		if s != "127.0.0.1" && s != "::1" && s != "127.0.0.53" {
			out = append(out, s)
		}
	}
	return out
}
