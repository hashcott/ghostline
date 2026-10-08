package netid

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
)

type windowsSource struct{}

// NewWindows reads the network identity through the IP Helper and WLAN APIs.
func NewWindows() Source { return windowsSource{} }

func (windowsSource) NetworkKey() string           { return networkKey() }
func (windowsSource) CurrentSSID() (string, error) { return winutil.CurrentSSID() }
func (windowsSource) WifiNames() ([]string, error) { return winutil.WifiNames() }
func (windowsSource) LiveAdapters() []LiveAdapter  { return liveAdapters() }

// adaptersAddresses returns the GetAdaptersAddresses list (with
// gateways), or nil.
func adaptersAddresses() *windows.IpAdapterAddresses {
	size := uint32(15 * 1024)
	var buf []byte
	for i := 0; i < 3; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0x80, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			return (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil
		}
	}
	return nil
}

// networkKey identifies the current network by the default gateway of the
// first connected adapter: its address and the router's hardware address,
// so two networks that both use 192.168.1.1 get their own server ranking.
// When the router's address cannot be read (IPv6-only), the adapter's own
// stands in.
func networkKey() string {
	first := adaptersAddresses()
	if first == nil {
		return "unknown"
	}
	for aa := first; aa != nil; aa = aa.Next {
		if aa.OperStatus != 1 || aa.FirstGatewayAddress == nil {
			continue
		}
		gw := aa.FirstGatewayAddress.Address.IP()
		for g := aa.FirstGatewayAddress; g != nil; g = g.Next {
			if ip := g.Address.IP(); ip.To4() != nil {
				gw = ip
				break
			}
		}
		if mac := gatewayMAC(gw); mac != "" {
			return scanner.NetworkKey(gw.String(), mac)
		}
		return scanner.NetworkKey(gw.String(), net.HardwareAddr(aa.PhysicalAddress[:aa.PhysicalAddressLength]).String())
	}
	return scanner.NetworkKey("none", "none")
}

var (
	procSendARP = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("SendARP")
	arpMu       sync.Mutex
	arpCache    = map[string]arpEntry{}
)

type arpEntry struct {
	mac string
	at  time.Time
}

// gatewayMAC returns the hardware address of an IPv4 gateway ("" when
// unknown), remembered for a minute: the key is read on every pick.
func gatewayMAC(ip net.IP) string {
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	arpMu.Lock()
	defer arpMu.Unlock()
	if e, ok := arpCache[ip4.String()]; ok && time.Since(e.at) < time.Minute {
		return e.mac
	}
	var mac [8]byte
	n := uint32(len(mac))
	dest := binary.LittleEndian.Uint32(ip4) // IPAddr is in network order
	r, _, _ := procSendARP.Call(uintptr(dest), 0, uintptr(unsafe.Pointer(&mac[0])), uintptr(unsafe.Pointer(&n)))
	out := ""
	if r == 0 && n >= 6 && n <= 8 {
		out = net.HardwareAddr(mac[:n]).String()
	}
	arpCache[ip4.String()] = arpEntry{mac: out, at: time.Now()}
	return out
}

// liveAdapters lists the DNS servers (static or DHCP) and gateway of every
// up adapter that has a gateway.
func liveAdapters() []LiveAdapter {
	var out []LiveAdapter
	for aa := adaptersAddresses(); aa != nil; aa = aa.Next {
		if aa.OperStatus != 1 || aa.FirstGatewayAddress == nil {
			continue
		}
		la := LiveAdapter{Gateway: aa.FirstGatewayAddress.Address.IP().String()}
		for d := aa.FirstDnsServerAddress; d != nil; d = d.Next {
			la.DNS = append(la.DNS, d.Address.IP().String())
		}
		out = append(out, la)
	}
	return out
}
