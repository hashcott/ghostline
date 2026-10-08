package netid

import (
	"net"
	"net/netip"
)

var privatePrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("fc00::/7"),
}

// LANAddrs lists the private (RFC 1918 / ULA) addresses of interfaces that
// are up, not loopback and on the LAN (onLAN: not a container or VM
// bridge) — what other devices can use to reach the proxy.
func LANAddrs(ifaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error), onLAN func(net.Interface) bool) []netip.Addr {
	var out []netip.Addr
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 || !onLAN(i) {
			continue
		}
		as, err := addrs(i)
		if err != nil {
			continue
		}
		for _, a := range as {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(n.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			for _, p := range privatePrefixes {
				if p.Contains(ip) {
					out = append(out, ip)
					break
				}
			}
		}
	}
	return out
}

// UnicastAddrs lists every unicast address (any scope) of interfaces that
// are up — what the proxy's SSRF guard treats as "this machine".
func UnicastAddrs(ifaces []net.Interface, addrs func(net.Interface) ([]net.Addr, error)) []netip.Addr {
	var out []netip.Addr
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		as, err := addrs(i)
		if err != nil {
			continue
		}
		for _, a := range as {
			n, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip, ok := netip.AddrFromSlice(n.IP); ok && !ip.IsMulticast() && !ip.IsUnspecified() {
				out = append(out, ip.Unmap())
			}
		}
	}
	return out
}

// LocalUnicastAddrs is UnicastAddrs for this machine.
func LocalUnicastAddrs() []netip.Addr {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	return UnicastAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return i.Addrs() })
}

// LocalLANAddrs is LANAddrs for this machine.
func LocalLANAddrs() []netip.Addr {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	return LANAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return i.Addrs() }, isOnLAN)
}

// IsPrivateOrLocal reports whether a is loopback, private (RFC 1918, ULA)
// or link-local: the only sources the LAN DNS server and the phone setup
// page answer.
func IsPrivateOrLocal(a netip.Addr) bool {
	a = a.Unmap()
	if a.IsLoopback() || a.IsLinkLocalUnicast() {
		return true
	}
	for _, p := range privatePrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
