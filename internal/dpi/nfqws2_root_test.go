//go:build linux && integration_root

package dpi_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	builtinStrategies "github.com/hashcott/ghostline/assets/strategies"
	zapret2Files "github.com/hashcott/ghostline/assets/zapret2"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/stretchr/testify/require"
)

func fetch(t *testing.T) error {
	t.Helper()
	c := &http.Client{Timeout: 10 * time.Second}
	resp, err := c.Get("https://example.com")
	if err == nil {
		resp.Body.Close()
	}
	return err
}

// The real engine behind the real table: traffic flows while it runs and
// after it is killed (queue bypass), and Stop leaves no table.
func TestNfqws2_FailOpen(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	l, err := strategies.Parse(builtinStrategies.BuiltinJSON, zapret2.ValidateArgs)
	require.NoError(t, err)
	list := func() strategies.List { return l }
	// nfqws2 drops to nobody: the test's temp dirs (0700) must be
	// traversable, as /var/lib is for the daemon.
	tmp := t.TempDir()
	require.NoError(t, os.Chmod(tmp, 0o755))
	require.NoError(t, os.Chmod(filepath.Dir(tmp), 0o755))
	root := filepath.Join(tmp, "ghostline")
	m := dpi.NewManager(filepath.Join(root, "bin"), []dpi.Installed{{Engine: zapret2.New(list), Assets: zapret2Files.FS}},
		dpi.NewLinuxRunner(), dpi.NewNftables(zapret2.Filter), nil)
	pid, err := m.Start(context.Background(), "zapret2", dpi.Plan{Strategy: l.Zapret2[0].ID, Scope: dpi.ScopeAll})
	require.NoError(t, err)
	defer func() { _ = m.Stop() }()
	require.NoError(t, fetch(t), "through nfqws2")

	require.NoError(t, syscall.Kill(pid, syscall.SIGKILL))
	require.Eventually(t, func() bool { return !m.Running() }, 2*time.Second, 50*time.Millisecond)
	require.NoError(t, fetch(t), "nfqws2 dead: the queue must let packets through")

	require.NoError(t, m.Stop())
	_, err = os.Stat("/proc/" + itoa(pid))
	require.True(t, os.IsNotExist(err))
}

func itoa(i int) string { return strconv.Itoa(i) }
