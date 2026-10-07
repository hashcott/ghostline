package sysdns

import (
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// The Linux backends reach the system through these small interfaces: the
// real ones speak D-Bus (dbus_linux.go), tests use fakes.

// nmAPI is NetworkManager.
type nmAPI interface {
	Running() bool
	DNSMode() (string, error)              // DnsManager.Mode: "none" means NM leaves DNS alone
	GlobalDNS() (model.NMGlobalDNS, error) // GlobalDnsConfiguration
	SetGlobalDNS(model.NMGlobalDNS) error
	DNSServers() ([]string, error) // DnsManager.Configuration[].nameservers, in order
	Watch(onChange func()) (stop func(), err error)
}

// resolvedAPI is systemd-resolved.
type resolvedAPI interface {
	Running() bool
	Links() ([]model.ResolvedLink, error) // links that have DNS servers
	SetDefaultRoute(ifindex int, on bool) error
	FlushCaches() error
	Watch(onChange func()) (stop func(), err error)
}

// unitAPI is systemd.
type unitAPI interface {
	Reload(unit string) error
	Stop(unit string, wait time.Duration) error
}
