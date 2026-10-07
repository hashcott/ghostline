package core

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/store"
)

var siteLocal = netip.MustParsePrefix("fec0::/10") // Windows' placeholder IPv6 DNS

// ispResolvers lists this PC's own DNS servers, before Ghostline: the
// static servers recorded in state.json while connected, else the live
// adapters' DNS. When neither gives a usable address (a DHCP adapter while
// connected), the default gateway is offered: home routers forward DNS to
// the ISP.
func ispResolvers(st store.State, live []netid.LiveAdapter) []string {
	out := []string{}
	add := func(s string) {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return
		}
		a = a.Unmap()
		if a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() || siteLocal.Contains(a) {
			return
		}
		if !slices.Contains(out, a.String()) {
			out = append(out, a.String())
		}
	}
	if st.Phase != store.PhaseClean {
		for _, s := range st.DNS.Windows {
			for _, v := range append(slices.Clone(s.IPv4.Servers), s.IPv6.Servers...) {
				add(v)
			}
		}
	}
	for _, a := range live {
		for _, v := range a.DNS {
			add(v)
		}
	}
	if len(out) == 0 {
		for _, a := range live {
			add(a.Gateway)
		}
	}
	// IPv4 first: an ISP's IPv6 resolver often does not answer.
	v6 := func(ip string) int {
		if strings.Contains(ip, ":") {
			return 1
		}
		return 0
	}
	slices.SortStableFunc(out, func(x, y string) int { return v6(x) - v6(y) })
	return out
}

// plainUpstream is plain UDP DNS to ip:53 (Ghostline's own engine on
// loopback, or the ISP comparison source of the lookup tool).
func plainUpstream(ip string) (upstream.Upstream, error) {
	return upstream.AddressToUpstream("udp://"+net.JoinHostPort(ip, "53"), &upstream.Options{Timeout: 5 * time.Second})
}

// dialDirect dials without Ghostline's proxy (clean-IP scan).
func dialDirect(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, addr)
}
