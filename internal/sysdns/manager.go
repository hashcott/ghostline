package sysdns

import (
	"errors"
	"log/slog"
	"slices"
	"strings"

	"github.com/hashcott/ghostline/internal/netwatch"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// Manager snapshots, sets and restores adapter DNS settings.
type Manager struct {
	api   API
	sleep func(time.Duration)
}

// NewManager wraps api; sleep is injectable for tests.
func NewManager(api API, sleep func(time.Duration)) *Manager {
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Manager{api: api, sleep: sleep}
}

// Select returns the adapters to manage. "auto" picks Ethernet and Wi-Fi
// adapters that are up and have a default gateway; "manual" picks guids
// that still exist.
func (m *Manager) Select(mode string, guids []string) ([]Adapter, error) {
	all, err := m.api.Adapters()
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, a := range all {
		if mode == "manual" {
			if slices.Contains(guids, a.GUID) {
				out = append(out, a)
			}
			continue
		}
		if (a.IfType == ifTypeEthernet || a.IfType == ifTypeWiFi) && a.Up && a.HasGateway {
			out = append(out, a)
		}
	}
	return out, nil
}

func family(servers []string) model.FamilyDNS {
	// Exactly loopback is Ghostline's own fingerprint, never an original
	// setting: recording it would make a later restore keep DNS dead.
	if len(servers) == 0 || slices.Equal(servers, []string{"127.0.0.1"}) || slices.Equal(servers, []string{"::1"}) {
		return model.FamilyDNS{Mode: model.DNSModeDHCP}
	}
	return model.FamilyDNS{Mode: model.DNSModeStatic, Servers: servers}
}

// Snapshot records the current DNS of each adapter.
func (m *Manager) Snapshot(ads []Adapter) ([]model.AdapterSnapshot, error) {
	out := make([]model.AdapterSnapshot, 0, len(ads))
	for _, a := range ads {
		s := model.AdapterSnapshot{GUID: a.GUID, LUID: a.LUID, IfIndex: a.IfIndex, Alias: a.Alias,
			IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}}
		v4, err := m.api.GetDNS(a.GUID, false)
		if err != nil {
			return nil, err
		}
		s.IPv4 = family(v4)
		if a.HasIPv6 {
			v6, err := m.api.GetDNS(a.GUID, true)
			if err != nil {
				return nil, err
			}
			s.IPv6 = family(v6)
		}
		out = append(out, s)
	}
	return out, nil
}

// ApplyLoopback points every snapshotted adapter at 127.0.0.1 (and ::1 when
// v6 is true and the adapter has IPv6). An adapter that cannot be set does
// not stop the others: the *ApplyError names it.
func (m *Manager) ApplyLoopback(snaps []model.AdapterSnapshot, v6 bool) error {
	ads, err := m.byGUID()
	if err != nil {
		return err
	}
	var failed []string
	var errs []error
	for _, s := range snaps {
		a, ok := ads[s.GUID]
		if !ok {
			continue
		}
		if err := m.setLoopback(a, v6); err != nil {
			failed, errs = append(failed, s.Alias), append(errs, err)
		}
	}
	if failed != nil {
		return &ApplyError{Failed: failed, Err: errors.Join(errs...)}
	}
	return nil
}

func (m *Manager) setLoopback(a Adapter, v6 bool) error {
	if err := m.api.SetDNS(a.GUID, false, []string{"127.0.0.1"}); err != nil {
		slog.Warn("sysdns: SetDNS loopback failed; trying netsh", "err", err,
			"adapter", a.Alias, "guid", a.GUID, "ifIndex", a.IfIndex, "family", "ipv4")
		if err := m.api.NetshSetDNS(a.IfIndex, false, []string{"127.0.0.1"}); err != nil {
			return err
		}
	}
	if v6 && a.HasIPv6 {
		if err := m.api.SetDNS(a.GUID, true, []string{"::1"}); err != nil {
			slog.Warn("sysdns: SetDNS loopback failed; trying netsh", "err", err,
				"adapter", a.Alias, "guid", a.GUID, "ifIndex", a.IfIndex, "family", "ipv6")
			if err := m.api.NetshSetDNS(a.IfIndex, true, []string{"::1"}); err != nil {
				return err
			}
		}
	}
	return nil
}

// ApplyLoopbackNetsh points the snapshotted adapters at loopback through
// netsh, by interface index. On Windows before 2004 (build 19041)
// SetInterfaceDnsSettings can succeed while the DNS client keeps using the
// old servers; netsh's change reaches it.
func (m *Manager) ApplyLoopbackNetsh(snaps []model.AdapterSnapshot, v6 bool) error {
	ads, err := m.byGUID()
	if err != nil {
		return err
	}
	var failed []string
	var errs []error
	for _, s := range snaps {
		a, ok := ads[s.GUID]
		if !ok {
			continue
		}
		err := m.api.NetshSetDNS(a.IfIndex, false, []string{"127.0.0.1"})
		if err == nil && v6 && a.HasIPv6 {
			err = m.api.NetshSetDNS(a.IfIndex, true, []string{"::1"})
		}
		if err != nil {
			failed, errs = append(failed, s.Alias), append(errs, err)
		}
	}
	if failed != nil {
		return &ApplyError{Failed: failed, Err: errors.Join(errs...)}
	}
	return nil
}

func (m *Manager) byGUID() (map[string]Adapter, error) {
	all, err := m.api.Adapters()
	if err != nil {
		return nil, err
	}
	out := make(map[string]Adapter, len(all))
	for _, a := range all {
		out[a.GUID] = a
	}
	return out, nil
}

// Restore puts each adapter back to its snapshot, keyed by GUID (aliases can
// change). Per family: SetDNS ×3, then netsh by interface index, then netsh
// DHCP. Adapters that no longer exist are skipped. The DNS cache is flushed.
func (m *Manager) Restore(snaps []model.AdapterSnapshot) []RestoreError {
	ads, err := m.byGUID()
	if err != nil {
		var out []RestoreError
		for _, s := range snaps {
			out = append(out, RestoreError{Target: s.Alias, Err: err})
		}
		return out
	}
	var out []RestoreError
	for _, s := range snaps {
		a, ok := ads[s.GUID]
		if !ok {
			continue
		}
		for _, f := range []struct {
			v6  bool
			dns model.FamilyDNS
		}{{false, s.IPv4}, {true, s.IPv6}} {
			if f.dns.Mode == "" || (f.v6 && !a.HasIPv6) {
				continue // nothing recorded for this family
			}
			if err := m.restoreFamily(a, f.v6, f.dns); err != nil {
				out = append(out, RestoreError{Target: s.Alias, Err: err})
			}
		}
	}
	if err := m.api.Flush(); err != nil {
		slog.Warn("sysdns: flush DNS cache after restore failed", "err", err)
	}
	return out
}

func (m *Manager) restoreFamily(a Adapter, v6 bool, f model.FamilyDNS) error {
	var servers []string
	if f.Mode == model.DNSModeStatic {
		servers = f.Servers
	}
	fam := "ipv4"
	if v6 {
		fam = "ipv6"
	}
	logFail := func(attempt int, method string, servers []string, err error) {
		slog.Warn("sysdns: restore attempt failed", "err", err, "adapter", a.Alias, "guid", a.GUID,
			"ifIndex", a.IfIndex, "family", fam, "attempt", attempt, "method", method,
			"mode", f.Mode, "servers", servers)
	}
	var err error
	for i := 0; i < 3; i++ {
		if i > 0 {
			m.sleep(200 * time.Millisecond)
		}
		if err = m.api.SetDNS(a.GUID, v6, servers); err == nil {
			return nil
		}
		logFail(i+1, "SetDNS", servers, err)
	}
	if err = m.api.NetshSetDNS(a.IfIndex, v6, servers); err == nil {
		return nil
	}
	logFail(4, "netsh", servers, err)
	if len(servers) > 0 {
		if err = m.api.NetshSetDNS(a.IfIndex, v6, nil); err == nil {
			slog.Warn("sysdns: restored DHCP instead of the recorded static servers",
				"adapter", a.Alias, "guid", a.GUID, "family", fam, "servers", servers)
			return nil
		}
		logFail(5, "netsh dhcp", nil, err)
	}
	return err
}

// LoopbackAdapters lists adapters whose DNS is exactly 127.0.0.1 or ::1 —
// the fingerprint Ghostline leaves when it cannot read its own state.
func (m *Manager) LoopbackAdapters() ([]Adapter, error) {
	all, err := m.api.Adapters()
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, a := range all {
		v4, err := m.api.GetDNS(a.GUID, false)
		if err != nil {
			slog.Warn("sysdns: read adapter DNS failed", "err", err, "adapter", a.Alias, "guid", a.GUID, "family", "ipv4")
		}
		v6, err := m.api.GetDNS(a.GUID, true)
		if err != nil && a.HasIPv6 {
			slog.Warn("sysdns: read adapter DNS failed", "err", err, "adapter", a.Alias, "guid", a.GUID, "family", "ipv6")
		}
		if slices.Equal(v4, []string{"127.0.0.1"}) || slices.Equal(v6, []string{"::1"}) {
			out = append(out, a)
		}
	}
	return out, nil
}

// AdapterDNS is an adapter with the DNS servers Windows reports for it.
type AdapterDNS struct {
	Adapter
	IPv4 []string
	IPv6 []string
}

// Report lists every adapter that is up, with its DNS servers. Windows'
// fec0:0:0:ffff::N placeholders (no IPv6 DNS configured) are left out.
func (m *Manager) Report() ([]AdapterDNS, error) {
	all, err := m.api.Adapters()
	if err != nil {
		return nil, err
	}
	var out []AdapterDNS
	for _, a := range all {
		if !a.Up || a.IfType == ifTypeLoopback {
			continue
		}
		r := AdapterDNS{Adapter: a}
		var err error
		if r.IPv4, err = m.api.GetDNS(a.GUID, false); err != nil {
			slog.Warn("sysdns: read adapter DNS failed", "err", err, "adapter", a.Alias, "guid", a.GUID, "family", "ipv4")
		}
		if a.HasIPv6 {
			v6, err := m.api.GetDNS(a.GUID, true)
			if err != nil {
				slog.Warn("sysdns: read adapter DNS failed", "err", err, "adapter", a.Alias, "guid", a.GUID, "family", "ipv6")
			}
			for _, s := range v6 {
				if !strings.HasPrefix(strings.ToLower(s), "fec0:0:0:ffff::") {
					r.IPv6 = append(r.IPv6, s)
				}
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// Flush clears the Windows DNS cache.
func (m *Manager) Flush() error { return m.api.Flush() }

// Debounce is netwatch.Debounce (kept here for the Windows watcher).
func Debounce(d time.Duration, f func()) (trigger func(), stop func()) {
	return netwatch.Debounce(d, f)
}
