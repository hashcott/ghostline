// Package sysdns reads and changes the DNS servers of Windows network adapters.
package sysdns

// Adapter is a network interface as seen by Ghostline.
type Adapter struct {
	GUID       string `json:"guid"`
	LUID       uint64 `json:"luid"`
	IfIndex    uint32 `json:"ifIndex"`
	Alias      string `json:"alias"`
	IfType     uint32 `json:"ifType"`
	Up         bool   `json:"up"`
	HasGateway bool   `json:"hasGateway"`
	HasIPv6    bool   `json:"hasIpv6"`
}

// API is the Win32 surface sysdns needs. An empty server list means
// "revert to DHCP".
type API interface {
	Adapters() ([]Adapter, error)
	GetDNS(guid string, v6 bool) ([]string, error)
	SetDNS(guid string, v6 bool, servers []string) error
	NetshSetDNS(ifIndex uint32, v6 bool, servers []string) error
	Flush() error
}

// RestoreError reports something whose DNS could not be restored: an
// adapter, "NetworkManager", "/etc/resolv.conf", ….
type RestoreError struct {
	Target string
	Err    error
}

func (e RestoreError) Error() string { return "sysdns: restore " + e.Target + ": " + e.Err.Error() }

const (
	ifTypeEthernet = 6
	ifTypeWiFi     = 71
	ifTypeLoopback = 24
)
