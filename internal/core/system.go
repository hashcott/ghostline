package core

import (
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/hashcott/ghostline/internal/procs"
)

// system implements app.System.
type system struct{ procs procs.Inspector }

func (s system) IsAdmin() bool { return s.procs.IsAdmin() }
func (s system) PortOwners(p uint16) ([]procs.PortOwner, error) {
	return s.procs.PortOwners(p)
}
func (s system) SelfPID() (uint32, time.Time) {
	pid := uint32(os.Getpid())
	start, _ := s.procs.StartTime(pid)
	return pid, start
}

// ListenFree binds UDP and TCP on each address the way the DNS engine will,
// then releases them.
func (system) ListenFree(addrs []netip.AddrPort) error {
	for _, a := range addrs {
		u, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(a))
		if err != nil {
			return err
		}
		t, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(a))
		u.Close()
		if err != nil {
			return err
		}
		t.Close()
	}
	return nil
}

// IPv6Available reports whether [::1] can be bound (IPv6 may be disabled).
func (system) IPv6Available() bool {
	c, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		return false
	}
	c.Close()
	return true
}
