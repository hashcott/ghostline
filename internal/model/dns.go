package model

// DNSSnapshot records the system's DNS configuration before Ghostline
// changed it, in the form of the backend that changed it. It is what
// state.json keeps so any process can put DNS back.
type DNSSnapshot struct {
	Backend string            `json:"backend"`
	Windows []AdapterSnapshot `json:"windows,omitempty"`
	Linux   *LinuxDNS         `json:"linux,omitempty"`
}

// LinuxDNS is what the Linux backends record (each fills its own part).
type LinuxDNS struct {
	NMGlobal      *NMGlobalDNS    `json:"nmGlobal,omitempty"`
	ResolvedLinks []ResolvedLink  `json:"resolvedLinks,omitempty"`
	DropIn        bool            `json:"dropIn,omitempty"`
	ResolvConf    *ResolvConfFile `json:"resolvConf,omitempty"`
	Servers       []string        `json:"servers,omitempty"` // in use before Ghostline
}

// NMGlobalDNS is NetworkManager's GlobalDnsConfiguration.
type NMGlobalDNS struct {
	Searches []string            `json:"searches,omitempty"`
	Options  []string            `json:"options,omitempty"`
	Domains  map[string]NMDomain `json:"domains,omitempty"`
}

// NMDomain is one domain of the global configuration ("*" is every domain).
type NMDomain struct {
	Servers []string `json:"servers,omitempty"`
	Options []string `json:"options,omitempty"`
}

// ResolvedLink is a systemd-resolved link that has DNS servers.
type ResolvedLink struct {
	IfIndex      int      `json:"ifIndex"`
	Name         string   `json:"name"`
	DefaultRoute bool     `json:"defaultRoute"`
	Servers      []string `json:"servers,omitempty"`
}

// ResolvConfFile is /etc/resolv.conf: a symlink, or contents and mode.
type ResolvConfFile struct {
	Symlink string `json:"symlink,omitempty"`
	Content []byte `json:"content,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
}

// Empty reports whether nothing was recorded.
func (s DNSSnapshot) Empty() bool { return len(s.Windows) == 0 && s.Linux == nil }

// Label names what the snapshot covers, for messages: the first adapter,
// else the backend.
func (s DNSSnapshot) Label() string {
	if len(s.Windows) > 0 {
		return s.Windows[0].Alias
	}
	return s.Backend
}

// Servers lists the DNS servers in use before Ghostline (the ISP lookup
// source): static adapter servers, or what the Linux backend found.
func (s DNSSnapshot) Servers() []string {
	out := []string{}
	for _, a := range s.Windows {
		out = append(out, a.IPv4.Servers...)
		out = append(out, a.IPv6.Servers...)
	}
	if s.Linux != nil {
		out = append(out, s.Linux.Servers...)
	}
	return out
}
