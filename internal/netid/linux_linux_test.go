package netid

import (
	"errors"
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/stretchr/testify/require"
)

const routeFixture = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eno1	00000000	0101A8C0	0003	0	0	200	00000000	0	0	0
wlan0	00000000	0100A8C0	0003	0	0	600	00000000	0	0	0
wlan0	0000A8C0	00000000	0001	0	0	600	00FFFFFF	0	0	0
`

const arpFixture = `IP address       HW type     Flags       HW address            Mask     Device
192.168.0.1      0x1         0x2         AA:BB:CC:DD:EE:01     *        wlan0
192.168.1.1      0x1         0x0         00:00:00:00:00:00     *        eno1
`

func TestDefaultRoute(t *testing.T) {
	iface, gw, ok := defaultRoute([]byte(routeFixture))
	require.True(t, ok)
	require.Equal(t, "eno1", iface, "the lowest metric wins")
	require.Equal(t, netip.MustParseAddr("192.168.1.1"), gw)
	_, _, ok = defaultRoute([]byte("Iface\tDestination\n"))
	require.False(t, ok)
}

func TestArpMAC(t *testing.T) {
	mac, ok := arpMAC([]byte(arpFixture), netip.MustParseAddr("192.168.0.1"))
	require.True(t, ok)
	require.Equal(t, "aa:bb:cc:dd:ee:01", mac)
	_, ok = arpMAC([]byte(arpFixture), netip.MustParseAddr("192.168.1.1"))
	require.False(t, ok, "incomplete entry")
}

type fakeProc struct {
	files  map[string]string
	poked  int
	ownMAC string
}

func (f *fakeProc) src(nm nmAPI) *linuxSource {
	return &linuxSource{nm: nm,
		read:     func(p string) ([]byte, error) { return []byte(f.files[p]), nil },
		poke:     func(netip.Addr) { f.poked++ },
		ifaceMAC: func(string) string { return f.ownMAC },
		sleep:    func() {}}
}

func TestNetworkKey_FromGatewayARP(t *testing.T) {
	f := &fakeProc{files: map[string]string{"/proc/net/route": routeFixture, "/proc/net/arp": arpFixture}}
	f.files["/proc/net/route"] = "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\nwlan0\t00000000\t0100A8C0\t0003\t0\t0\t600\n"
	require.Equal(t, scanner.NetworkKey("192.168.0.1", "aa:bb:cc:dd:ee:01"), f.src(nil).NetworkKey())
	require.Zero(t, f.poked)
}

func TestNetworkKey_FallsBackToOwnMAC(t *testing.T) {
	f := &fakeProc{files: map[string]string{"/proc/net/route": routeFixture, "/proc/net/arp": arpFixture}, ownMAC: "11:22:33:44:55:66"}
	require.Equal(t, scanner.NetworkKey("192.168.1.1", "11:22:33:44:55:66"), f.src(nil).NetworkKey())
	require.Equal(t, 1, f.poked, "asked the gateway once to fill the ARP table")
}

func TestNetworkKey_NoRouteIsNone(t *testing.T) {
	f := &fakeProc{files: map[string]string{}}
	require.Equal(t, scanner.NetworkKey("none", "none"), f.src(nil).NetworkKey())
}

func TestLiveAdapters(t *testing.T) {
	f := &fakeProc{files: map[string]string{"/proc/net/route": routeFixture}}
	require.Equal(t, []LiveAdapter{{Gateway: "192.168.1.1"}, {Gateway: "192.168.0.1"}}, f.src(nil).LiveAdapters())
}

type fakeNM struct {
	ssid  string
	saved []string
}

func (n fakeNM) ActiveSSID() (string, error)    { return n.ssid, nil }
func (n fakeNM) SavedSSIDs() ([]string, error) { return n.saved, nil }

func TestSSID_FromFakeNM(t *testing.T) {
	s := (&fakeProc{}).src(fakeNM{ssid: "Home", saved: []string{"Home", "Cafe"}})
	got, err := s.CurrentSSID()
	require.NoError(t, err)
	require.Equal(t, "Home", got)
	names, err := s.WifiNames()
	require.NoError(t, err)
	require.Equal(t, []string{"Home", "Cafe"}, names)
}

func TestSSID_NoNM(t *testing.T) {
	_, err := (&fakeProc{}).src(nil).CurrentSSID()
	require.True(t, errors.Is(err, errors.ErrUnsupported))
}
