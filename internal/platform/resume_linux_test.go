package platform

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

// Review I2: only logind's own PrepareForSleep(false) re-checks the engine.
func TestWatchResume_OnlyLogind(t *testing.T) {
	addr := privateBus(t)
	dial := func() *dbus.Conn {
		c, err := dbus.Connect(addr)
		require.NoError(t, err)
		t.Cleanup(func() { c.Close() })
		return c
	}
	logind, spoof, watcher := dial(), dial(), dial()
	reply, err := logind.RequestName(logindName, dbus.NameFlagDoNotQueue)
	require.NoError(t, err)
	require.Equal(t, dbus.RequestNameReplyPrimaryOwner, reply)

	var hits atomic.Int32
	stop, err := watchResumeOn(watcher, func() { hits.Add(1) })
	require.NoError(t, err)
	defer stop()

	emit := func(c *dbus.Conn) {
		require.NoError(t, c.Emit("/org/freedesktop/login1", "org.freedesktop.login1.Manager.PrepareForSleep", false))
	}
	emit(spoof)
	time.Sleep(200 * time.Millisecond)
	require.Zero(t, hits.Load())
	emit(logind)
	require.Eventually(t, func() bool { return hits.Load() == 1 }, 2*time.Second, 10*time.Millisecond)
}
