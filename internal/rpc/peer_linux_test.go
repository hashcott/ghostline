package rpc

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pair returns the server end of a real Unix-socket connection.
func pair(t *testing.T) net.Conn {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "p.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = l.Close() })
	got := make(chan net.Conn, 1)
	go func() { c, _ := l.Accept(); got <- c }()
	cl, err := net.Dial("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cl.Close() })
	srv := <-got
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func TestAllowGroups_SelfViaExtraUID(t *testing.T) {
	require.NoError(t, AllowGroups(nil, os.Getuid())(pair(t)))
}

func TestAllowGroups_RejectsWhenNotListed(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root is always allowed")
	}
	err := AllowGroups([]string{"no-such-group-xyz"})(pair(t))
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), CodeNotAuthorized), err.Error())
}
