package shell

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"time"

	"github.com/hashcott/ghostline/internal/startup"
	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// system implements app.System.
type system struct{}

func (system) IsAdmin() bool { return winutil.IsAdmin() }
func (system) PortOwners(p uint16) ([]winutil.PortOwner, error) {
	return winutil.PortOwners(p)
}
func (system) SelfPID() (uint32, time.Time) {
	pid := uint32(os.Getpid())
	start, _ := winutil.ProcessStartTime(pid)
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

// safety implements app.Safety.
type safety struct{ exe string }

func (s safety) StartWatchdog(pid uint32, start time.Time) (func() error, error) {
	// Deliberately not in our job object, and broken away from any job we
	// inherited from a terminal or IDE: it must outlive us.
	cmd, err := winutil.StartDetached(s.exe, []string{"--watchdog", "--parent", strconv.FormatUint(uint64(pid), 10),
		"--parent-start", strconv.FormatInt(start.UnixNano(), 10)})
	if err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	return func() error { return cmd.Process.Kill() }, nil
}

func (s safety) CreateRecoveryTask() error { return startup.Create(startup.RecoveryTask(s.exe)) }
func (s safety) DeleteRecoveryTask() error { return startup.Delete(startup.RecoveryTask(s.exe).Name) }

const webView2ClientKey = `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

func webView2Installed() bool {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, path := range []string{webView2ClientKey, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`} {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			v, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && v != "" && v != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

func messageBox(title, text string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	_, _ = windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONWARNING)
}

func fatalBox(err error) {
	messageBox("Ghostline", fmt.Sprintf("Ghostline không thể khởi động / could not start:\n\n%v", err))
}
