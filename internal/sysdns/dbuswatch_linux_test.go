package sysdns

import (
	"bufio"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

// privateBus starts a throwaway dbus-daemon and returns its address.
func privateBus(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("no dbus-daemon")
	}
	cmd := exec.Command(bin, "--session", "--nofork", "--print-address")
	out, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	require.NoError(t, err)
	return strings.TrimSpace(line)
}

func dial(t *testing.T, addr string) *dbus.Conn {
	t.Helper()
	c, err := dbus.Connect(addr)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	return c
}

func propsChanged(path dbus.ObjectPath, dest string) *dbus.Message {
	m := &dbus.Message{Type: dbus.TypeSignal, Headers: map[dbus.HeaderField]dbus.Variant{
		dbus.FieldPath:      dbus.MakeVariant(path),
		dbus.FieldInterface: dbus.MakeVariant("org.freedesktop.DBus.Properties"),
		dbus.FieldMember:    dbus.MakeVariant("PropertiesChanged"),
		dbus.FieldSignature: dbus.MakeVariant(dbus.SignatureOf("", map[string]dbus.Variant{}, []string{})),
	}, Body: []any{"x", map[string]dbus.Variant{}, []string{}}}
	if dest != "" {
		m.Headers[dbus.FieldDestination] = dbus.MakeVariant(dest)
	}
	return m
}

// Review I2: any user may send signals on the system bus. Only the service
// that owns the name counts, and a burst is one re-check.
func TestWatchProps_OnlyTheOwnerDebounced(t *testing.T) {
	addr := privateBus(t)
	owner, spoof, watcher := dial(t, addr), dial(t, addr), dial(t, addr)
	reply, err := owner.RequestName(nmName, dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	require.Equal(t, dbus.RequestNameReplyPrimaryOwner, reply)

	var hits atomic.Int32
	stop, err := watchProps(watcher, nmName, []dbus.ObjectPath{nmPath}, 50*time.Millisecond, func() { hits.Add(1) })
	require.NoError(t, err)
	defer stop()

	send := func(c *dbus.Conn, m *dbus.Message) { require.NoError(t, c.Send(m, nil).Err) }
	send(spoof, propsChanged(nmPath, ""))
	send(spoof, propsChanged(nmPath, watcher.Names()[0])) // unicast, past the match rules
	time.Sleep(300 * time.Millisecond)
	require.Zero(t, hits.Load(), "a signal from someone else is ignored")

	for range 5 {
		send(owner, propsChanged(nmPath, ""))
	}
	require.Eventually(t, func() bool { return hits.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int32(1), hits.Load(), "a burst is one call")
}
