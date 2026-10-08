package dpi

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

var testFilter = []CaptureRule{
	{Proto: "tcp", Ports: []int{80, 443}, OutPackets: 20, InPackets: 10},
	{Proto: "udp", Ports: []int{443}, OutPackets: 5, InPackets: 3, QUIC: true},
}

func TestNftPlan_Queues(t *testing.T) {
	var post, pre []nftRule
	for _, r := range nftPlan(testFilter) {
		if r.Action != "queue" {
			continue
		}
		if r.Chain == "post" {
			post = append(post, r)
		} else {
			pre = append(pre, r)
		}
	}
	require.Equal(t, []nftRule{
		{Chain: "post", Action: "queue", Proto: "tcp", Port: 80, Dir: "original", Packets: 20},
		{Chain: "post", Action: "queue", Proto: "tcp", Port: 443, Dir: "original", Packets: 20},
		{Chain: "post", Action: "queue", Proto: "udp", Port: 443, Dir: "original", Packets: 5},
	}, post)
	require.Equal(t, []nftRule{
		{Chain: "pre", Action: "queue", Proto: "tcp", Port: 80, Dir: "reply", Packets: 10},
		{Chain: "pre", Action: "queue", Proto: "tcp", Port: 443, Dir: "reply", Packets: 10},
		{Chain: "pre", Action: "queue", Proto: "udp", Port: 443, Dir: "reply", Packets: 3},
	}, pre)
}

// Nothing reaches a queue rule before the engine's own packets, loopback
// and local networks are let through.
func TestNftPlan_SkipsLanAndOwnMark(t *testing.T) {
	plan := nftPlan(testFilter)
	require.Equal(t, nftRule{Chain: "post", Action: "markaccept"}, plan[0])
	require.Equal(t, nftRule{Chain: "post", Action: "loaccept"}, plan[1])
	var nets []netip.Prefix
	firstQueue := -1
	for i, r := range plan {
		if r.Action == "netaccept" {
			require.Equal(t, -1, firstQueue, "skip nets come before any queue rule")
			nets = append(nets, r.Net)
			if r.Net.Addr().Is4() {
				require.Equal(t, "ip", r.Family)
			} else {
				require.Equal(t, "ip6", r.Family)
			}
		}
		if r.Action == "queue" && firstQueue < 0 {
			firstQueue = i
		}
	}
	for _, s := range []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "100.64.0.0/10", "::1/128", "fe80::/10", "fc00::/7"} {
		require.Contains(t, nets, netip.MustParsePrefix(s))
	}
}

func TestNftPlan_TTLGuard(t *testing.T) {
	var ttl []nftRule
	for _, r := range nftPlan(testFilter) {
		if r.Action == "ttldrop" {
			ttl = append(ttl, r)
		}
	}
	require.Equal(t, []nftRule{{Chain: "pre", Action: "ttldrop", Family: "ip"}, {Chain: "pre", Action: "ttldrop", Family: "ip6"}}, ttl)
}
