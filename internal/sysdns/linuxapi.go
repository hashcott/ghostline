package sysdns

import (
	"errors"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// The Linux backends reach the system through these small interfaces: the
// real ones speak D-Bus (dbus_linux.go), tests use fakes.

// nmAPI is NetworkManager.
type nmAPI interface {
	Running() bool
	DNSMode() (string, error)              // DnsManager.Mode: "none" means NM leaves DNS alone
	RcManager() (string, error)            // DnsManager.RcManager: "unmanaged" means NM does not write resolv.conf
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

// errNoSuchLink is SetDefaultRoute on a link resolved no longer knows (a
// VPN that went down): there is nothing left to restore on it.
var errNoSuchLink = errors.New("resolved: no such link")

// unitAPI is systemd.
type unitAPI interface {
	Reload(unit string) error
	Stop(unit string, wait time.Duration) error
}
