package core

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/certstore"
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

type nopEmitter struct{}

func (nopEmitter) Emit(string, any) {}

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

// noAdapters is a system DNS with nothing of Ghostline's to reset.
type noAdapters struct{ sysdns.UnsupportedBackend }

func (noAdapters) RestoreDefault() error { return nil }

// watchedDNS counts Watch registrations and stops.
type watchedDNS struct {
	sysdns.UnsupportedBackend
	watch func(func()) (func(), error)
}

func (w watchedDNS) Watch(f func()) (func(), error) { return w.watch(f) }

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newCore(t *testing.T, p platform.Deps, o Options) *Core {
	t.Helper()
	o.Platform, o.Log = p, quietLog()
	if o.Emitter == nil {
		o.Emitter = nopEmitter{}
	}
	c, err := New(o)
	require.NoError(t, err)
	return c
}

func TestNew_ServesSettingsAndSnapshot(t *testing.T) {
	c := newCore(t, testDeps(t), Options{})
	require.Equal(t, store.DefaultSettings().Language, c.Svc.GetSettings().Language)
	require.Equal(t, app.StatusDisconnected, c.Svc.GetSnapshot().Status)
}

// Review Focus 1: a crashed run's state.json is dealt with before anything
// else exists, and a corrupt one says so in the first snapshot.
func TestNew_RestoresOrphanedStateFirst(t *testing.T) {
	p := testDeps(t)
	require.NoError(t, os.MkdirAll(p.Paths.DataDir, 0o755))
	require.NoError(t, store.WriteJSONAtomic(p.Paths.State, store.State{Version: 3, Phase: store.PhaseDNSSet, PID: 999999999}))
	newCore(t, p, Options{})
	st, err := store.NewStateStore(p.Paths.State, &memLock{}).Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, st.Phase)

	p = testDeps(t)
	p.DNS = noAdapters{} // a machine with nothing to reset, so the corrupt state ends clean
	require.NoError(t, os.MkdirAll(p.Paths.DataDir, 0o755))
	require.NoError(t, os.WriteFile(p.Paths.State, []byte("{bad"), 0o644))
	c := newCore(t, p, Options{})
	var codes []string
	for _, w := range c.Svc.GetSnapshot().Warnings {
		codes = append(codes, w.Code)
	}
	require.Contains(t, codes, app.CodeStateReset)
}

func TestStart_WaitReturnsWhenCancelled(t *testing.T) {
	c := newCore(t, testDeps(t), Options{})
	ctx, cancel := context.WithCancel(context.Background())
	wait := c.Start(ctx)
	done := make(chan struct{})
	go func() { wait(); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background loops did not stop after cancel")
	}
}

// The watches are in place when Start returns (the Windows autostart
// connect runs right after it), and stopped once wait returns.
func TestStart_WatchesRegisteredBeforeReturn(t *testing.T) {
	p := testDeps(t)
	var mu sync.Mutex
	registered, stopped := 0, 0
	p.DNS = watchedDNS{watch: func(func()) (func(), error) {
		mu.Lock()
		registered++
		mu.Unlock()
		return func() { mu.Lock(); stopped++; mu.Unlock() }, nil
	}}
	c := newCore(t, p, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	wait := c.Start(ctx)
	mu.Lock()
	require.Equal(t, 1, registered)
	mu.Unlock()
	cancel()
	wait()
	mu.Lock()
	require.Equal(t, 1, stopped)
	mu.Unlock()
}

func TestNew_CallsSettingsHookAfterSave(t *testing.T) {
	var got []string
	c := newCore(t, testDeps(t), Options{OnSettingsChanged: func(old, n store.Settings) { got = append(got, old.Language+">"+n.Language) }})
	s := c.Svc.GetSettings()
	s.Language = "en"
	require.NoError(t, c.Svc.SaveSettings(s))
	require.Equal(t, []string{"vi>en"}, got)
}

// C2: the daemon serves other users as root; it refuses lists that name a
// file on its machine, while the Windows GUI (its own user) keeps them.
func TestNew_RemoteRefusesFileLists(t *testing.T) {
	file := lists.List{Name: "x", Source: "file", Path: "/etc/shadow", Format: "hosts", Action: "block"}
	c := newCore(t, testDeps(t), Options{Remote: true})
	_, err := c.Svc.AddList(file)
	require.ErrorIs(t, err, lists.ErrFileListsOff)

	// Not remote: file lists pass the gate and reach the usual checks. An
	// empty path fails there, synchronously, so no background fetch
	// outlives the test (Windows cannot remove a directory still in use).
	file.Path = ""
	c = newCore(t, testDeps(t), Options{})
	_, err = c.Svc.AddList(file)
	require.Error(t, err)
	require.NotErrorIs(t, err, lists.ErrFileListsOff)
}

// After sleep the engine is re-checked (Linux: logind's PrepareForSleep).
func TestStart_RegistersResumeWatch(t *testing.T) {
	p := testDeps(t)
	var onResume func()
	stopped := 0
	p.WatchResume = func(f func()) (func(), error) {
		onResume = f
		return func() { stopped++ }, nil
	}
	c := newCore(t, p, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	wait := c.Start(ctx)
	require.NotNil(t, onResume)
	onResume() // disconnected: nothing to re-check, must not panic
	cancel()
	wait()
	require.Equal(t, 1, stopped)
}

// A login runs the proxy work that waited for a desktop session.
func TestStart_RegistersSessionWatch(t *testing.T) {
	p := testDeps(t)
	var onNew func()
	stopped := 0
	p.WatchSessions = func(f func()) (func(), error) {
		onNew = f
		return func() { stopped++ }, nil
	}
	c := newCore(t, p, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	wait := c.Start(ctx)
	require.NotNil(t, onNew)
	onNew() // not connected: nothing to reapply, must not panic
	cancel()
	wait()
	require.Equal(t, 1, stopped)
}
