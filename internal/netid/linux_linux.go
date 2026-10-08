package netid

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/scanner"
)

// nmAPI is NetworkManager's Wi-Fi state (nm_linux.go).
type nmAPI interface {
	ActiveSSID() (string, error)
	SavedSSIDs() ([]string, error)
}

// linuxSource reads the default route and ARP table from /proc, and Wi-Fi
// from NetworkManager (nil: none).
type linuxSource struct {
	nm       nmAPI
	read     func(path string) ([]byte, error)
	poke     func(gw netip.Addr) // makes the kernel ARP for gw
	ifaceMAC func(iface string) string
	sleep    func()
}

// NewLinux identifies networks from /proc and NetworkManager.
func NewLinux() Source {
	var nm nmAPI
	if n, err := newNM(); err == nil {
		nm = n
	}
	return &linuxSource{nm: nm, read: os.ReadFile, poke: pokeGateway, ifaceMAC: interfaceMAC,
		sleep: func() { time.Sleep(200 * time.Millisecond) }}
}

type route struct {
	iface  string
	gw     netip.Addr
	metric int
}

// defaultRoutes lists the IPv4 default routes in /proc/net/route order.
func defaultRoutes(procRoute []byte) []route {
	var out []route
	sc := bufio.NewScanner(bytes.NewReader(procRoute))
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 7 || f[1] != "00000000" {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		var ip [4]byte
		binary.LittleEndian.PutUint32(ip[:], binary.BigEndian.Uint32(b)) // kernel order → address bytes
		m, _ := strconv.Atoi(f[6])
		out = append(out, route{iface: f[0], gw: netip.AddrFrom4(ip), metric: m})
	}
	return out
}

// defaultRoute is the default route with the lowest metric.
func defaultRoute(procRoute []byte) (string, netip.Addr, bool) {
	rs := defaultRoutes(procRoute)
	if len(rs) == 0 {
		return "", netip.Addr{}, false
	}
	best := rs[0]
	for _, r := range rs[1:] {
		if r.metric < best.metric {
			best = r
		}
	}
	return best.iface, best.gw, true
}

// arpMAC is ip's hardware address from a complete /proc/net/arp entry.
func arpMAC(procArp []byte, ip netip.Addr) (string, bool) {
	sc := bufio.NewScanner(bytes.NewReader(procArp))
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 4 || f[0] != ip.String() {
			continue
		}
		if flags, err := strconv.ParseUint(strings.TrimPrefix(f[2], "0x"), 16, 32); err != nil || flags&0x2 == 0 {
			return "", false
		}
		return strings.ToLower(f[3]), true
	}
	return "", false
}

func pokeGateway(gw netip.Addr) {
	c, err := net.DialUDP("udp4", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(gw, 9)))
	if err != nil {
		return
	}
	_, _ = c.Write([]byte{0})
	_ = c.Close()
}

func interfaceMAC(iface string) string {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return ""
	}
	return ifc.HardwareAddr.String()
}

// NetworkKey is the default gateway and its MAC; without an ARP entry the
// gateway is asked once, then this machine's own MAC stands in (Windows does
// the same).
func (s *linuxSource) NetworkKey() string {
	b, _ := s.read("/proc/net/route")
	iface, gw, ok := defaultRoute(b)
	if !ok {
		return scanner.NetworkKey("none", "none")
	}
	arp, _ := s.read("/proc/net/arp")
	mac, ok := arpMAC(arp, gw)
	if !ok {
		s.poke(gw)
		s.sleep()
		arp, _ = s.read("/proc/net/arp")
		if mac, ok = arpMAC(arp, gw); !ok {
			mac = s.ifaceMAC(iface)
		}
	}
	return scanner.NetworkKey(gw.String(), mac)
}

// LiveAdapters lists the interfaces with a default route. Their DNS is not
// per interface on Linux: the DNS snapshot carries the ISP's servers.
func (s *linuxSource) LiveAdapters() []LiveAdapter {
	b, _ := s.read("/proc/net/route")
	var out []LiveAdapter
	for _, r := range defaultRoutes(b) {
		out = append(out, LiveAdapter{Gateway: r.gw.String()})
	}
	return out
}

func (s *linuxSource) CurrentSSID() (string, error) {
	if s.nm == nil {
		return "", errUnsupported
	}
	return s.nm.ActiveSSID()
}

func (s *linuxSource) WifiNames() ([]string, error) {
	if s.nm == nil {
		return nil, errUnsupported
	}
	return s.nm.SavedSSIDs()
}
