package core

import (
	"errors"
	"log/slog"
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
	start, err := s.procs.StartTime(pid)
	if err != nil {
		slog.Warn("core: reading this process's start time failed", "pid", pid, "err", err)
	}
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

// LoopbackUDP binds 127.0.0.1:port, sends itself a datagram and waits
// for it. A program that redirects DNS (AdGuard, an antivirus, a VPN)
// takes such a datagram to port 53 before it arrives.
func (system) LoopbackUDP(port uint16) error {
	srv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: int(port)})
	if err != nil {
		return err
	}
	defer srv.Close()
	c, err := net.DialUDP("udp4", nil, srv.LocalAddr().(*net.UDPAddr))
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err := c.Write([]byte("ghostline-probe")); err != nil {
		return err
	}
	if err := srv.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	buf := make([]byte, 64)
	n, _, err := srv.ReadFromUDP(buf)
	if err != nil {
		return err
	}
	if string(buf[:n]) != "ghostline-probe" {
		return errors.New("core: loopback probe: unexpected datagram")
	}
	return nil
}

func (s system) ProcessNames() ([]string, error) { return s.procs.ProcessNames() }

// IPv6Available reports whether [::1] can be bound (IPv6 may be disabled).
func (system) IPv6Available() bool {
	c, err := net.ListenPacket("udp6", "[::1]:0")
	if err != nil {
		return false
	}
	c.Close()
	return true
}
