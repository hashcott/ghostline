package netid

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLANAddrs(t *testing.T) {
	ifs := []net.Interface{
		{Index: 1, Name: "Wi-Fi", Flags: net.FlagUp},
		{Index: 2, Name: "Loopback", Flags: net.FlagUp | net.FlagLoopback},
		{Index: 3, Name: "Down", Flags: 0},
	}
	addrs := map[int][]net.Addr{
		1: {mustCIDR("192.168.1.5/24"), mustCIDR("8.8.8.8/32"), mustCIDR("fe80::1/64"), mustCIDR("fd00::5/64")},
		2: {mustCIDR("127.0.0.1/8")},
		3: {mustCIDR("10.0.0.9/8")},
	}
	got := LANAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return addrs[i.Index], nil }, func(net.Interface) bool { return true })
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("fd00::5")}, got)
}

// A container or VM bridge (docker0 172.17.0.1) is private but no other
// device on the LAN can reach it: it is not offered.
func TestLANAddrs_SkipsInterfacesOffTheLAN(t *testing.T) {
	ifs := []net.Interface{
		{Index: 1, Name: "wlan0", Flags: net.FlagUp},
		{Index: 2, Name: "docker0", Flags: net.FlagUp},
	}
	addrs := map[int][]net.Addr{1: {mustCIDR("192.168.2.115/24")}, 2: {mustCIDR("172.17.0.1/16")}}
	got := LANAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return addrs[i.Index], nil },
		func(i net.Interface) bool { return i.Name != "docker0" })
	require.Equal(t, []netip.Addr{netip.MustParseAddr("192.168.2.115")}, got)
}

func mustCIDR(s string) net.Addr {
	ip, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	n.IP = ip
	return n
}

// Final review I6: the SSRF guard needs every address of this machine,
// including public IPv4 and global IPv6, not only the private ones.
func TestUnicastAddrs_IncludesPublicAndGlobal(t *testing.T) {
	ifs := []net.Interface{{Index: 1, Name: "Wi-Fi", Flags: net.FlagUp}, {Index: 2, Name: "Down", Flags: 0}}
	addrs := map[int][]net.Addr{
		1: {mustCIDR("192.168.1.5/24"), mustCIDR("203.0.113.7/24"), mustCIDR("2405:4800::5/64"), mustCIDR("fe80::1/64"), mustCIDR("100.64.1.2/10")},
		2: {mustCIDR("10.0.0.9/8")},
	}
	got := UnicastAddrs(ifs, func(i net.Interface) ([]net.Addr, error) { return addrs[i.Index], nil })
	require.ElementsMatch(t, []netip.Addr{
		netip.MustParseAddr("192.168.1.5"), netip.MustParseAddr("203.0.113.7"), netip.MustParseAddr("2405:4800::5"),
		netip.MustParseAddr("fe80::1"), netip.MustParseAddr("100.64.1.2"),
	}, got)
}

func TestIsPrivateOrLocal(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.5", "fd00::1", "169.254.1.1", "fe80::1"} {
		require.True(t, IsPrivateOrLocal(netip.MustParseAddr(s)), s)
	}
	for _, s := range []string{"8.8.8.8", "172.32.0.1", "2001:4860::8888", "100.64.0.1"} {
		require.False(t, IsPrivateOrLocal(netip.MustParseAddr(s)), s)
	}
	require.True(t, IsPrivateOrLocal(netip.MustParseAddr("::ffff:192.168.1.5")))
}
