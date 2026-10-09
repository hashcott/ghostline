package winutil

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseUDP4Table(t *testing.T) {
	// dwNumEntries=2, rows of {localAddr, localPort, owningPid}
	b := make([]byte, 4+2*12)
	binary.LittleEndian.PutUint32(b[0:], 2)
	binary.LittleEndian.PutUint32(b[4:], 0x0100007f) // 127.0.0.1 in memory order
	b[8], b[9] = 0x00, 0x35                          // port 53, network byte order
	binary.LittleEndian.PutUint32(b[12:], 4321)
	binary.LittleEndian.PutUint32(b[16:], 0) // 0.0.0.0
	b[20], b[21] = 0x1f, 0x90                // 8080
	binary.LittleEndian.PutUint32(b[24:], 99)
	rows := parseUDP4Table(b)
	require.Len(t, rows, 2)
	require.Equal(t, uint16(53), rows[0].port)
	require.Equal(t, uint32(4321), rows[0].pid)
	require.Equal(t, "127.0.0.1", rows[0].addr.String())
	require.Equal(t, uint16(8080), rows[1].port)
}

func pids(owners []PortOwner) []uint32 {
	var out []uint32
	for _, o := range owners {
		out = append(out, o.PID)
	}
	return out
}

func TestPortOwners_FindsOwnUDPListener(t *testing.T) {
	c, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer c.Close()
	owners, err := PortOwners(uint16(c.LocalAddr().(*net.UDPAddr).Port))
	require.NoError(t, err)
	require.Contains(t, pids(owners), uint32(os.Getpid()))
	for _, o := range owners {
		if o.PID == uint32(os.Getpid()) {
			require.Equal(t, "udp", o.Proto)
			require.NotEmpty(t, o.Name)
		}
	}
}

func TestPortOwners_FindsOwnTCPListener(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	owners, err := PortOwners(uint16(l.Addr().(*net.TCPAddr).Port))
	require.NoError(t, err)
	require.Contains(t, pids(owners), uint32(os.Getpid()))
}

func TestProcessAlive(t *testing.T) {
	pid := uint32(os.Getpid())
	start, err := ProcessStartTime(pid)
	require.NoError(t, err)
	require.True(t, ProcessAlive(pid, start))
	require.False(t, ProcessAlive(pid, start.Add(time.Second)))
	require.False(t, ProcessAlive(0xFFFFFFF0, start))
}

func TestProcessNames_IncludesThisTest(t *testing.T) {
	names, err := ProcessNames()
	require.NoError(t, err)
	self, err := os.Executable()
	require.NoError(t, err)
	require.Contains(t, names, filepath.Base(self))
}

func TestJob_KillsChildOnClose(t *testing.T) {
	cmd := HiddenCmd("cmd", []string{"/c", "ping -n 30 127.0.0.1"}, "")
	require.NoError(t, cmd.Start())
	j, err := NewKillOnCloseJob()
	require.NoError(t, err)
	require.NoError(t, j.Assign(cmd.Process))
	require.NoError(t, j.Close())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("child still running after job close")
	}
}

func TestNamedMutex_Exclusive(t *testing.T) {
	name := `Local\Ghostline-Test-` + time.Now().Format("150405.000000")
	m1, err := NewNamedMutex(name)
	require.NoError(t, err)
	m2, err := NewNamedMutex(name)
	require.NoError(t, err)
	require.NoError(t, m1.Lock())
	var wg sync.WaitGroup
	got := make(chan time.Time, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		require.NoError(t, m2.Lock())
		got <- time.Now()
		require.NoError(t, m2.Unlock())
	}()
	time.Sleep(150 * time.Millisecond)
	select {
	case <-got:
		t.Fatal("second handle acquired the mutex while the first held it")
	default:
	}
	require.NoError(t, m1.Unlock())
	wg.Wait()
	require.Len(t, got, 1)
}

func TestIsAdmin_DoesNotPanic(t *testing.T) { _ = IsAdmin() }

func TestServiceRunning_UnknownService(t *testing.T) {
	running, err := ServiceRunning("Ghostline-Does-Not-Exist")
	require.NoError(t, err)
	require.False(t, running)
}

func TestNamedMutex_ExclusiveWithinOneInstance(t *testing.T) { // review I6
	m, err := NewNamedMutex(`Local\Ghostline-Test-Same-` + time.Now().Format("150405.000000"))
	require.NoError(t, err)
	require.NoError(t, m.Lock())
	got := make(chan struct{})
	go func() {
		_ = m.Lock()
		close(got)
		_ = m.Unlock()
	}()
	select {
	case <-got:
		t.Fatal("second goroutine acquired the mutex while it was held")
	case <-time.After(150 * time.Millisecond):
	}
	require.NoError(t, m.Unlock())
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second goroutine never acquired the mutex")
	}
}

func TestFindServices_NoMatch(t *testing.T) {
	names, err := FindServices("Ghostline-No-Such-Service-")
	require.NoError(t, err)
	require.Empty(t, names)
}

func TestFindServices_FindsKnownDriver(t *testing.T) {
	names, err := FindServices("Tcpip") // the TCP/IP driver exists on every Windows
	require.NoError(t, err)
	require.Contains(t, names, "Tcpip")
}

func TestCurrentSSID_NoPanic(t *testing.T) {
	ssid, err := CurrentSSID()
	require.NoError(t, err)
	require.LessOrEqual(t, len(ssid), 32)
}

func TestWifiNames_NoPanic(t *testing.T) {
	names, err := WifiNames()
	require.NoError(t, err)
	for _, n := range names {
		require.NotEmpty(t, n)
		require.LessOrEqual(t, len(n), 32)
	}
}
