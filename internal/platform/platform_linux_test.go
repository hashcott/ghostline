package platform

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewDev_PutsEverythingUnderDir(t *testing.T) {
	dir := t.TempDir()
	d, err := NewDev(dir)
	require.NoError(t, err)
	for _, p := range []string{d.Paths.DataDir, d.Paths.LogDir, d.Paths.State, d.Socket, d.Lock.(*fileLock).path} {
		require.True(t, strings.HasPrefix(p, dir+string(filepath.Separator)), p)
	}
	require.True(t, d.UsesDaemon)
}

func TestNew_DaemonDirectories(t *testing.T) {
	d, err := New("/usr/lib/ghostline/ghostlined")
	require.NoError(t, err)
	require.Equal(t, "/var/lib/ghostline/data", d.Paths.DataDir)
	require.Equal(t, "/var/log/ghostline", d.Paths.LogDir)
	require.Equal(t, "/run/ghostline/ctl.sock", d.Socket)
	require.Equal(t, "/run/ghostline/state.lock", d.Lock.(*fileLock).path)
}

func TestClientSocket_EnvOverride(t *testing.T) {
	t.Setenv("GHOSTLINE_SOCKET", "")
	require.Equal(t, "/run/ghostline/ctl.sock", ClientSocket())
	t.Setenv("GHOSTLINE_SOCKET", "/tmp/x.sock")
	require.Equal(t, "/tmp/x.sock", ClientSocket())
}
