package headless

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/secrets"
	"github.com/hashcott/ghostline/internal/startup"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

type memLock struct{ mu sync.Mutex }

func (l *memLock) Lock() error   { l.mu.Lock(); return nil }
func (l *memLock) Unlock() error { l.mu.Unlock(); return nil }

// testDeps is an OS without system support, with data in a temp directory.
func testDeps(t *testing.T) platform.Deps {
	t.Helper()
	dir := t.TempDir()
	return platform.Deps{
		Paths: store.ResolvePaths(filepath.Join(dir, "ghostline"), dir),
		Lock:  &memLock{},

		DNS:      sysdns.UnsupportedBackend{},
		SysProxy: sysproxy.Unsupported{},
		Certs:    certstore.Unsupported{}, Firewall: firewall.Unsupported{},

		DPIRunner: dpi.UnsupportedRunner{}, DPIInterceptor: dpi.NoInterceptor{},
		DPIEngines: func(func() strategies.List) []dpi.Installed { return nil },

		Startup:       startup.Unsupported{},
		StartWatchdog: func(uint32, time.Time) (func() error, error) { return func() error { return nil }, nil },

		UserSecrets: secrets.Unsupported{}, MachineSecrets: secrets.Unsupported{},
		NetID: netid.Unsupported{}, Procs: procs.Unsupported{},
		AttachConsole: func() {},
	}
}

func TestRun_ExportWritesFile(t *testing.T) {
	p := testDeps(t)
	out := filepath.Join(t.TempDir(), "out.json")
	require.Equal(t, 0, Run(cli.Mode{Kind: cli.KindExport, ExportPath: out}, p))
	b, err := os.ReadFile(out)
	require.NoError(t, err)
	_, err = backup.Parse(b, backup.Data{Settings: store.DefaultSettings()}, backup.Validators{Settings: func(store.Settings) error { return nil }, List: func(lists.List) error { return nil }})
	require.NoError(t, err)
}

func TestRun_RestoreOnCleanStateIsNoop(t *testing.T) {
	p := testDeps(t)
	require.Equal(t, 0, Run(cli.Mode{Kind: cli.KindRestore}, p))
	st, err := store.NewStateStore(p.Paths.State, &memLock{}).Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, st.Phase)
}

// cleanupIC records Cleanup: on Linux that deletes the nftables table.
type cleanupIC struct {
	dpi.NoInterceptor
	cleaned int
}

func (c *cleanupIC) Cleanup() error { c.cleaned++; return nil }

// After kill -9 of a daemon that ran DPI, --restore (systemd's
// ExecStopPost) removes the packet capture it left behind.
func TestRun_RestoreAfterKillCleansCapture(t *testing.T) {
	p := testDeps(t)
	ic := &cleanupIC{}
	p.DPIInterceptor = ic
	st := store.CleanState()
	st.Phase, st.PID = store.PhaseDNSSet, 999999999
	st.DPI = store.DPIState{Running: true, PID: 999999998}
	require.NoError(t, os.MkdirAll(p.Paths.DataDir, 0o755))
	require.NoError(t, store.WriteJSONAtomic(p.Paths.State, st))
	require.Equal(t, 0, Run(cli.Mode{Kind: cli.KindRestore}, p))
	require.Equal(t, 1, ic.cleaned)
	got, err := store.NewStateStore(p.Paths.State, &memLock{}).Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, got.Phase)
}
