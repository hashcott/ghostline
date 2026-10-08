//go:build linux

package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Review Focus 2: a killed daemon leaves its socket file behind.
func TestListen_ReplacesStaleSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "run", "ctl.sock")
	require.NoError(t, os.MkdirAll(filepath.Dir(sock), 0o755))
	require.NoError(t, os.WriteFile(sock, []byte("stale"), 0o600))
	l, err := listen(sock)
	require.NoError(t, err)
	defer l.Close()
	fi, err := os.Stat(sock)
	require.NoError(t, err)
	require.Equal(t, os.ModeSocket, fi.Mode().Type())
	require.Equal(t, os.FileMode(0o666), fi.Mode().Perm())
}

func TestListen_RefusesWhenLive(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	live, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer live.Close()
	go func() {
		for {
			c, err := live.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, err = listen(sock)
	require.Error(t, err)
}
