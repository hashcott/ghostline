package dpi

import "net/netip"

// nftRule describes one rule of the Linux capture table; nft_linux.go turns
// it into netlink expressions. Keeping the plan pure lets it be tested on
// any OS.
type nftRule struct {
	Chain   string       // "post" (postrouting) | "pre" (prerouting)
	Family  string       // "ip" | "ip6" | "" (both)
	Action  string       // markaccept | loaccept | netaccept | queue | ttldrop
	Net     netip.Prefix // netaccept: destination network let through
	Proto   string       // queue: "tcp" | "udp"
	Port    int          // queue: remote port (dport going out, sport coming back)
	Dir     string       // queue: conntrack direction, "original" | "reply"
	Packets int          // queue: the first N packets of that direction
}

// skipNets are never sent to the engine: loopback, private, link-local and
// carrier-grade NAT ranges.
var skipNets = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fc00::/7"),
}

// nftPlan lays out the table (zapret2's pre-NAT layout): in postrouting,
// the engine's own packets (fwmark; their conntrack entry is marked so ICMP
// errors they cause can be dropped), loopback and local networks pass, then
// the first packets of each captured connection are queued; in prerouting,
// the first replies are queued (auto-detection of blocked sites) and TTL
// errors caused by the engine's packets are dropped.
func nftPlan(f []CaptureRule) []nftRule {
	plan := []nftRule{{Chain: "post", Action: "markaccept"}, {Chain: "post", Action: "loaccept"}}
	for _, n := range skipNets {
		fam := "ip"
		if n.Addr().Is6() {
			fam = "ip6"
		}
		plan = append(plan, nftRule{Chain: "post", Family: fam, Action: "netaccept", Net: n})
	}
	for _, r := range f {
		for _, p := range r.Ports {
			plan = append(plan, nftRule{Chain: "post", Action: "queue", Proto: r.Proto, Port: p, Dir: "original", Packets: r.OutPackets})
		}
	}
	for _, r := range f {
		for _, p := range r.Ports {
			plan = append(plan, nftRule{Chain: "pre", Action: "queue", Proto: r.Proto, Port: p, Dir: "reply", Packets: r.InPackets})
		}
	}
	return append(plan, nftRule{Chain: "pre", Action: "ttldrop", Family: "ip"}, nftRule{Chain: "pre", Action: "ttldrop", Family: "ip6"})
}
