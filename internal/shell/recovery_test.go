package shell

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

// Review Focus 1: both the GUI and the headless --restore/--watchdog paths
// use this builder; a missing hook leaves firewall rules or Fake SNI roots
// behind after a crash.
func TestRecoveryDeps_WiresEveryCleanup(t *testing.T) {
	p := platform.Deps{DNS: sysdns.Unsupported{}, SysProxy: sysproxy.Unsupported{}, Certs: certstore.Unsupported{},
		Firewall: firewall.Unsupported{}, Procs: procs.Unsupported{}}
	d := RecoveryDeps(p, store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), nil), func() error { return nil }, slog.Default())
	require.NotNil(t, d.DNS)
	require.NotNil(t, d.Alive)
	require.NotNil(t, d.StopDPI)
	require.NotNil(t, d.RestoreSysProxy)
	require.NotNil(t, d.DeleteRule)
	require.NotNil(t, d.RemoveCert)
	require.NotNil(t, d.SweepSession)
}
