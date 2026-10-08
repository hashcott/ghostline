package sysdns

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/netwatch"
)

const (
	nmName    = "org.freedesktop.NetworkManager"
	nmPath    = "/org/freedesktop/NetworkManager"
	nmDNSPath = "/org/freedesktop/NetworkManager/DnsManager"
	nmDNSIfc  = "org.freedesktop.NetworkManager.DnsManager"

	resolvedName = "org.freedesktop.resolve1"
	resolvedPath = "/org/freedesktop/resolve1"
	resolvedMgr  = "org.freedesktop.resolve1.Manager"

	systemdName = "org.freedesktop.systemd1"
	systemdPath = "/org/freedesktop/systemd1"
	systemdMgr  = "org.freedesktop.systemd1.Manager"
)

// dbusAPIs is the real system: one system-bus connection for all three.
type dbusAPIs struct {
	NM       nmAPI
	Resolved resolvedAPI
	Units    unitAPI
}

func newDBus() (*dbusAPIs, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &dbusAPIs{NM: &dbusNM{conn}, Resolved: &dbusResolved{conn}, Units: &dbusUnits{conn}}, nil
}

func hasOwner(conn *dbus.Conn, name string) bool {
	var ok bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, name).Store(&ok); err != nil {
		return false
	}
	return ok
}

// nameOwner is the unique connection name that owns name ("" if none).
func nameOwner(conn *dbus.Conn, name string) string {
	var owner string
	_ = conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner)
	return owner
}

// watchProps reports PropertiesChanged on the given paths of name, once
// delay after a burst settles. Any user may send signals on the system
// bus, so only those from the name's current owner count: the match rule
// filters broadcasts, and the sender check also drops signals sent
// straight to this connection.
func watchProps(conn *dbus.Conn, name string, paths []dbus.ObjectPath, delay time.Duration, onChange func()) (func(), error) {
	var opts [][]dbus.MatchOption
	for _, p := range paths {
		o := []dbus.MatchOption{dbus.WithMatchSender(name), dbus.WithMatchObjectPath(p), dbus.WithMatchInterface("org.freedesktop.DBus.Properties"), dbus.WithMatchMember("PropertiesChanged")}
		if err := conn.AddMatchSignal(o...); err != nil {
			return nil, err
		}
		opts = append(opts, o)
	}
	ch := make(chan *dbus.Signal, 16)
	conn.Signal(ch)
	want := map[dbus.ObjectPath]bool{}
	for _, p := range paths {
		want[p] = true
	}
	trigger, cancel := netwatch.Debounce(delay, onChange)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-ch:
				if s != nil && want[s.Path] && s.Name == "org.freedesktop.DBus.Properties.PropertiesChanged" && s.Sender != "" && s.Sender == nameOwner(conn, name) {
					trigger()
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		conn.RemoveSignal(ch)
		for _, o := range opts {
			_ = conn.RemoveMatchSignal(o...)
		}
		close(done)
		cancel()
	}, nil
}

// --- NetworkManager ---

type dbusNM struct{ conn *dbus.Conn }

func (n *dbusNM) Running() bool { return hasOwner(n.conn, nmName) }

func (n *dbusNM) DNSMode() (string, error) {
	v, err := n.conn.Object(nmName, nmDNSPath).GetProperty(nmDNSIfc + ".Mode")
	if err != nil {
		return "", err
	}
	s, _ := v.Value().(string)
	return s, nil
}

func (n *dbusNM) GlobalDNS() (model.NMGlobalDNS, error) {
	v, err := n.conn.Object(nmName, nmPath).GetProperty(nmName + ".GlobalDnsConfiguration")
	if err != nil {
		return model.NMGlobalDNS{}, err
	}
	m, _ := v.Value().(map[string]dbus.Variant)
	return variantToGlobal(m), nil
}

func (n *dbusNM) SetGlobalDNS(g model.NMGlobalDNS) error {
	return n.conn.Object(nmName, nmPath).SetProperty(nmName+".GlobalDnsConfiguration", dbus.MakeVariant(globalToVariant(g)))
}

func (n *dbusNM) DNSServers() ([]string, error) {
	v, err := n.conn.Object(nmName, nmDNSPath).GetProperty(nmDNSIfc + ".Configuration")
	if err != nil {
		return nil, err
	}
	entries, _ := v.Value().([]map[string]dbus.Variant)
	var out []string
	for _, e := range entries {
		if ns, ok := e["nameservers"].Value().([]string); ok {
			out = append(out, ns...)
		}
	}
	return out, nil
}

func (n *dbusNM) Watch(onChange func()) (func(), error) {
	return watchProps(n.conn, nmName, []dbus.ObjectPath{nmPath, nmDNSPath}, netwatch.Delay, onChange)
}

// globalToVariant writes GlobalDnsConfiguration: {"searches": as,
// "options": as, "domains": a{sv} of {"servers": as, "options": as}}.
func globalToVariant(g model.NMGlobalDNS) map[string]dbus.Variant {
	out := map[string]dbus.Variant{}
	if len(g.Searches) > 0 {
		out["searches"] = dbus.MakeVariant(g.Searches)
	}
	if len(g.Options) > 0 {
		out["options"] = dbus.MakeVariant(g.Options)
	}
	if len(g.Domains) > 0 {
		doms := map[string]dbus.Variant{}
		for name, d := range g.Domains {
			dv := map[string]dbus.Variant{}
			if len(d.Servers) > 0 {
				dv["servers"] = dbus.MakeVariant(d.Servers)
			}
			if len(d.Options) > 0 {
				dv["options"] = dbus.MakeVariant(d.Options)
			}
			doms[name] = dbus.MakeVariant(dv)
		}
		out["domains"] = dbus.MakeVariant(doms)
	}
	return out
}

func strs(v dbus.Variant) []string {
	s, _ := v.Value().([]string)
	return s
}

// variantToGlobal reads GlobalDnsConfiguration back.
func variantToGlobal(m map[string]dbus.Variant) model.NMGlobalDNS {
	g := model.NMGlobalDNS{}
	if v, ok := m["searches"]; ok {
		g.Searches = strs(v)
	}
	if v, ok := m["options"]; ok {
		g.Options = strs(v)
	}
	if v, ok := m["domains"]; ok {
		if doms, ok := v.Value().(map[string]dbus.Variant); ok && len(doms) > 0 {
			g.Domains = map[string]model.NMDomain{}
			for name, dv := range doms {
				dm, _ := dv.Value().(map[string]dbus.Variant)
				d := model.NMDomain{}
				if s, ok := dm["servers"]; ok {
					d.Servers = strs(s)
				}
				if o, ok := dm["options"]; ok {
					d.Options = strs(o)
				}
				g.Domains[name] = d
			}
		}
	}
	return g
}

// --- systemd-resolved ---

type dbusResolved struct{ conn *dbus.Conn }

func (r *dbusResolved) Running() bool { return hasOwner(r.conn, resolvedName) }

func (r *dbusResolved) mgr() dbus.BusObject { return r.conn.Object(resolvedName, resolvedPath) }

// Links lists links that have DNS servers (Manager.DNS: a(iiay)), with
// their DefaultRoute.
func (r *dbusResolved) Links() ([]model.ResolvedLink, error) {
	v, err := r.mgr().GetProperty(resolvedMgr + ".DNS")
	if err != nil {
		return nil, err
	}
	rows, ok := v.Value().([][]any)
	if !ok {
		return nil, errors.New("resolved: unexpected DNS property")
	}
	links := linksFromDNS(rows, ifaceInfo)
	for i := range links {
		var path dbus.ObjectPath
		if err := r.mgr().Call(resolvedMgr+".GetLink", 0, int32(links[i].IfIndex)).Store(&path); err != nil {
			return nil, err
		}
		dr, err := r.conn.Object(resolvedName, path).GetProperty("org.freedesktop.resolve1.Link.DefaultRoute")
		if err != nil {
			return nil, err
		}
		links[i].DefaultRoute, _ = dr.Value().(bool)
	}
	return links, nil
}

func (r *dbusResolved) SetDefaultRoute(ifindex int, on bool) error {
	err := r.mgr().Call(resolvedMgr+".SetLinkDefaultRoute", 0, int32(ifindex), on).Err
	var de dbus.Error
	if errors.As(err, &de) && de.Name == "org.freedesktop.resolve1.NoSuchLink" {
		return errNoSuchLink
	}
	return err
}

func (r *dbusResolved) FlushCaches() error { return r.mgr().Call(resolvedMgr+".FlushCaches", 0).Err }

func (r *dbusResolved) Watch(onChange func()) (func(), error) {
	return watchProps(r.conn, resolvedName, []dbus.ObjectPath{resolvedPath}, netwatch.Delay, onChange)
}

// --- systemd ---

type dbusUnits struct{ conn *dbus.Conn }

func (u *dbusUnits) mgr() dbus.BusObject { return u.conn.Object(systemdName, systemdPath) }

func (u *dbusUnits) activeState(unit string) (string, error) {
	var path dbus.ObjectPath
	if err := u.mgr().Call(systemdMgr+".GetUnit", 0, unit).Store(&path); err != nil {
		return "", err
	}
	v, err := u.conn.Object(systemdName, path).GetProperty("org.freedesktop.systemd1.Unit.ActiveState")
	if err != nil {
		return "", err
	}
	s, _ := v.Value().(string)
	return s, nil
}

// waitState polls ActiveState until done says so or wait passes.
func (u *dbusUnits) waitState(unit string, wait time.Duration, done func(string) bool) error {
	deadline := time.Now().Add(wait)
	for {
		st, err := u.activeState(unit)
		if err == nil && done(st) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("systemd: %s is %q after %s", unit, st, wait)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Reload makes unit re-read its configuration: a reload where the unit
// supports one, a restart otherwise (systemd-resolved before systemd 256,
// e.g. Ubuntu 24.04: "Job type reload is not applicable").
func (u *dbusUnits) Reload(unit string) error {
	if err := u.mgr().Call(systemdMgr+".ReloadOrRestartUnit", 0, unit, "replace").Err; err != nil {
		return err
	}
	return u.waitState(unit, 5*time.Second, func(s string) bool { return s == "active" })
}

func (u *dbusUnits) Stop(unit string, wait time.Duration) error {
	if err := u.mgr().Call(systemdMgr+".StopUnit", 0, unit, "replace").Err; err != nil {
		return err
	}
	return u.waitState(unit, wait, func(s string) bool { return s == "inactive" || s == "failed" })
}

func (n *dbusNM) RcManager() (string, error) {
	v, err := n.conn.Object(nmName, nmDNSPath).GetProperty(nmDNSIfc + ".RcManager")
	if err != nil {
		return "", err
	}
	s, _ := v.Value().(string)
	return s, nil
}

// linksFromDNS groups Manager.DNS rows (ifindex, family, address) by link.
// Global servers (ifindex 0), loopback interfaces and loopback servers are
// left out: they are Ghostline's own (resolved reports the global ::1 on
// lo) or a local stub, never a link's upstream to route around.
func linksFromDNS(rows [][]any, iface func(ifindex int) (name string, loopback bool)) []model.ResolvedLink {
	byIdx := map[int]*model.ResolvedLink{}
	for _, row := range rows {
		if len(row) != 3 {
			continue
		}
		idx, _ := row[0].(int32)
		raw, _ := row[2].([]byte)
		a, ok := netip.AddrFromSlice(raw)
		if idx <= 0 || !ok || a.Unmap().IsLoopback() {
			continue
		}
		l := byIdx[int(idx)]
		if l == nil {
			name, loopback := iface(int(idx))
			if loopback {
				continue
			}
			l = &model.ResolvedLink{IfIndex: int(idx), Name: name}
			byIdx[int(idx)] = l
		}
		l.Servers = append(l.Servers, a.Unmap().String())
	}
	out := make([]model.ResolvedLink, 0, len(byIdx))
	for _, l := range byIdx {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IfIndex < out[j].IfIndex })
	return out
}

// ifaceInfo names a link and says whether it is a loopback interface.
func ifaceInfo(ifindex int) (string, bool) {
	ifc, err := net.InterfaceByIndex(ifindex)
	if err != nil {
		return "", false
	}
	return ifc.Name, ifc.Flags&net.FlagLoopback != 0
}
