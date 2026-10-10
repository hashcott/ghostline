package sysdns

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// dnsInterfaceSettings mirrors DNS_INTERFACE_SETTINGS (netioapi.h), 64 bytes.
type dnsInterfaceSettings struct {
	Version             uint32
	_                   [4]byte
	Flags               uint64
	Domain              *uint16
	NameServer          *uint16
	SearchList          *uint16
	RegistrationEnabled uint32
	RegisterAdapterName uint32
	EnableLLMNR         uint32
	QueryAdapterName    uint32
	ProfileNameServer   *uint16
}

const (
	dnsSettingsVersion1  = 1
	dnsSettingIPv6       = 0x1
	dnsSettingNameServer = 0x2
	gaaFlagIncludeGW     = 0x80
	ifOperStatusUp       = 1
)

var (
	iphlpapi                     = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetInterfaceDnsSettings  = iphlpapi.NewProc("GetInterfaceDnsSettings")
	procSetInterfaceDnsSettings  = iphlpapi.NewProc("SetInterfaceDnsSettings")
	procFreeInterfaceDnsSettings = iphlpapi.NewProc("FreeInterfaceDnsSettings")
	dnsapi                       = windows.NewLazySystemDLL("dnsapi.dll")
	procDnsFlushResolverCache    = dnsapi.NewProc("DnsFlushResolverCache")
)

// errNoDNSSettingsAPI: this Windows has no SetInterfaceDnsSettings
// (documented from Windows 10 2004, build 19041). The Manager then sets DNS
// through netsh.
var errNoDNSSettingsAPI = errors.New("sysdns: SetInterfaceDnsSettings is not available on this Windows")

// haveDNSSettingsAPI reports whether iphlpapi.dll has the DNS settings
// calls. Calling a missing LazyProc panics, so every use checks first.
var haveDNSSettingsAPI = func() bool {
	return procGetInterfaceDnsSettings.Find() == nil && procSetInterfaceDnsSettings.Find() == nil &&
		procFreeInterfaceDnsSettings.Find() == nil
}

type winAPI struct{}

// NewWindowsAPI returns the real Win32 implementation of API.
func NewWindowsAPI() API { return winAPI{} }

func (winAPI) Adapters() ([]Adapter, error) {
	size := uint32(15 * 1024)
	var buf []byte
	for i := 0; i < 5; i++ {
		buf = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, gaaFlagIncludeGW, 0,
			(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == nil {
			break
		}
		if !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			return nil, fmt.Errorf("sysdns: GetAdaptersAddresses: %w", err)
		}
		if i == 4 {
			return nil, err
		}
	}
	var out []Adapter
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
		guid := windows.BytePtrToString(aa.AdapterName)
		if _, err := windows.GUIDFromString(guid); err != nil {
			continue
		}
		idx := aa.IfIndex
		if idx == 0 {
			idx = aa.Ipv6IfIndex
		}
		out = append(out, Adapter{
			GUID: guid, LUID: aa.Luid, IfIndex: idx,
			Alias:      windows.UTF16PtrToString(aa.FriendlyName),
			IfType:     aa.IfType,
			Up:         aa.OperStatus == ifOperStatusUp,
			HasGateway: aa.FirstGatewayAddress != nil,
			HasIPv6:    aa.Ipv6IfIndex != 0,
		})
	}
	return out, nil
}

func guidOf(s string) (windows.GUID, error) { return windows.GUIDFromString(s) }

func (winAPI) GetDNS(guid string, v6 bool) ([]string, error) {
	g, err := guidOf(guid)
	if err != nil {
		return nil, err
	}
	if !haveDNSSettingsAPI() {
		return registryDNS(guid, v6)
	}
	s := dnsInterfaceSettings{Version: dnsSettingsVersion1}
	if v6 {
		s.Flags = dnsSettingIPv6
	}
	r, _, _ := procGetInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&s)))
	if r != 0 {
		return nil, fmt.Errorf("sysdns: GetInterfaceDnsSettings: %w", windows.Errno(r))
	}
	defer func() { _, _, _ = procFreeInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&s))) }()
	if s.NameServer == nil {
		return nil, nil
	}
	return splitNameServers(windows.UTF16PtrToString(s.NameServer)), nil
}

func (winAPI) SetDNS(guid string, v6 bool, servers []string) error {
	g, err := guidOf(guid)
	if err != nil {
		return err
	}
	ns, err := windows.UTF16PtrFromString(strings.Join(servers, ","))
	if err != nil {
		return err
	}
	if !haveDNSSettingsAPI() {
		return errNoDNSSettingsAPI
	}
	s := dnsInterfaceSettings{Version: dnsSettingsVersion1, Flags: dnsSettingNameServer, NameServer: ns}
	if v6 {
		s.Flags |= dnsSettingIPv6
	}
	r, _, _ := procSetInterfaceDnsSettings.Call(uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&s)))
	if r != 0 {
		return fmt.Errorf("sysdns: SetInterfaceDnsSettings: %w", windows.Errno(r))
	}
	return nil
}

// registryDNS reads an adapter's static DNS servers where Windows keeps
// them (and where netsh writes them), for Windows without
// GetInterfaceDnsSettings. No NameServer value means DHCP.
func registryDNS(guid string, v6 bool) ([]string, error) {
	svc := "Tcpip"
	if v6 {
		svc = "Tcpip6"
	}
	path := `SYSTEM\CurrentControlSet\Services\` + svc + `\Parameters\Interfaces\` + guid
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sysdns: open %s: %w", path, err)
	}
	defer k.Close()
	v, _, err := k.GetStringValue("NameServer")
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sysdns: read %s NameServer: %w", path, err)
	}
	return splitNameServers(v), nil
}

func (winAPI) NetshSetDNS(ifIndex uint32, v6 bool, servers []string) error {
	run := func(args []string) error {
		out, err := winutil.HiddenCmd("netsh", args, "").CombinedOutput()
		if err != nil {
			return fmt.Errorf("sysdns: netsh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := run(netshArgs(ifIndex, v6, servers)); err != nil {
		return err
	}
	fam := "ipv4"
	if v6 {
		fam = "ipv6"
	}
	for i, s := range servers[min(1, len(servers)):] {
		args := []string{"interface", fam, "add", "dnsservers", "name=" + strconv.Itoa(int(ifIndex)),
			"address=" + s, "index=" + strconv.Itoa(i+2), "validate=no"}
		if err := run(args); err != nil {
			return err
		}
	}
	return nil
}

// netshArgs sets the primary server (or DHCP) by interface index, never by
// name: localized adapter names break netsh parsing.
func netshArgs(ifIndex uint32, v6 bool, servers []string) []string {
	fam := "ipv4"
	if v6 {
		fam = "ipv6"
	}
	base := []string{"interface", fam, "set", "dnsservers", "name=" + strconv.Itoa(int(ifIndex))}
	if len(servers) == 0 {
		return append(base, "source=dhcp")
	}
	return append(base, "source=static", "address="+servers[0], "register=primary", "validate=no")
}

func (winAPI) Flush() error {
	r, _, err := procDnsFlushResolverCache.Call()
	if r == 0 {
		return fmt.Errorf("sysdns: DnsFlushResolverCache: %w", err)
	}
	return nil
}

func splitNameServers(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
}
