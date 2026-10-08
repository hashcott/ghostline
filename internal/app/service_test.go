package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

type fEmitter struct {
	mu     sync.Mutex
	events map[string][]any
}

func (e *fEmitter) Emit(name string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.events == nil {
		e.events = map[string][]any{}
	}
	e.events[name] = append(e.events[name], data)
}

func (e *fEmitter) count(name string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.events[name])
}

type fScans struct{ results []scanner.Result }

func (f *fScans) Rescan(_ context.Context, on func(int, int, scanner.Result)) ([]scanner.Result, error) {
	for i, r := range f.results {
		if on != nil {
			on(i+1, len(f.results), r)
		}
	}
	return f.results, nil
}
func (f *fScans) Results() []scanner.Result { return f.results }

type svcHarness struct {
	*harness
	svc    *Service
	em     *fEmitter
	bus    *Bus
	custom []model.Server
	paths  store.Paths
	box    *SettingsBox
}

func newSvc(t *testing.T) *svcHarness {
	t.Helper()
	h := newHarness(t)
	dir := t.TempDir()
	paths := store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), dir)
	sh := &svcHarness{harness: h, em: &fEmitter{}, paths: paths}
	sh.box = NewSettingsBox(paths.Settings, h.settings)
	h.o.d.Settings = sh.box.Get
	h.o.d.SaveSettings = sh.box.Save
	h.o.d.BlacklistPath = paths.DPIBlacklist
	sh.bus = NewBus(sh.em)
	h.o.d.Sink = sh.bus
	h.o.d.Scans = &fScans{results: []scanner.Result{{ServerID: "cf", OK: true, Latency: 20 * time.Millisecond}}}
	sh.svc = NewService(h.o, ServiceDeps{
		Bus: sh.bus, Paths: paths, Settings: sh.box,
		Catalog: func() []model.Server {
			return append([]model.Server{{ID: "cf", Name: "Cloudflare", Source: model.SourceBuiltin}}, sh.custom...)
		},
		LoadCustom:   func() ([]model.Server, error) { return sh.custom, nil },
		SaveCustom:   func(s []model.Server) error { sh.custom = s; return nil },
		ListAdapters: func() ([]sysdns.Adapter, error) { return nil, nil },
		StopService:  func(string) error { return nil },
		SetMode:      func(string) {},
		RestoreNow:   func() error { return nil },
		Info:         func() AppInfo { return AppInfo{Version: "test"} },
	})
	// Background work (list downloads, proxy re-apply) must finish before
	// TempDir cleanup, or Windows refuses to delete files still in use.
	t.Cleanup(sh.svc.waitBackground)
	return sh
}

func TestLogBuffer_RingKeepsLastN(t *testing.T) {
	b := NewLogBuffer(3)
	for i := 0; i < 5; i++ {
		b.Add(LogEvent{Code: string(rune('a' + i))})
	}
	got := b.All()
	require.Equal(t, []string{"c", "d", "e"}, []string{got[0].Code, got[1].Code, got[2].Code})
}

func TestService_QueryLogOffByDefaultAndRAMOnly(t *testing.T) {
	s := newSvc(t)
	s.bus.Query(engine.QueryEvent{Domain: "secret.example."})
	require.Zero(t, s.em.count(EventQuery))
	require.Empty(t, s.svc.GetQueries())

	s.svc.SetQueryLog(true)
	for i := 0; i < 600; i++ {
		s.bus.Query(engine.QueryEvent{Domain: "secret.example."})
	}
	require.Len(t, s.svc.GetQueries(), 500)
	require.Equal(t, 600, s.em.count(EventQuery))

	// Nothing about queries is written to disk.
	entries, _ := os.ReadDir(s.paths.DataDir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(s.paths.DataDir, e.Name()))
		require.NotContains(t, string(b), "secret.example")
	}
	s.svc.SetQueryLog(false)
	require.Empty(t, s.svc.GetQueries(), "turning it off drops the buffer")
}

func TestService_SaveSettingsValidates(t *testing.T) {
	s := newSvc(t)
	bad := []func(*store.Settings){
		func(x *store.Settings) { x.MaxUpstreams = 0 },
		func(x *store.Settings) { x.MaxUpstreams = 11 },
		func(x *store.Settings) { x.Bootstrap = nil },
		func(x *store.Settings) { x.Bootstrap = []string{"nope"} },
		func(x *store.Settings) { x.DPI.Preset = "custom"; x.DPI.CustomArgs = "--dns-addr x" },
		func(x *store.Settings) { x.Language = "fr" },
	}
	for i, mut := range bad {
		st := store.DefaultSettings()
		mut(&st)
		require.Error(t, s.svc.SaveSettings(st), "case %d", i)
	}
	st := store.DefaultSettings()
	st.Language = "en"
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, "en", s.svc.GetSettings().Language)
	onDisk, _, _ := store.LoadSettings(s.paths.Settings)
	require.Equal(t, "en", onDisk.Language)
}

func TestService_AddServers(t *testing.T) {
	s := newSvc(t)
	n, bad := s.svc.AddServers("https://dns.example/dns-query\nudp://1.1.1.1\nhttps://dns.example/dns-query\n")
	require.Equal(t, 1, n)
	require.Equal(t, []string{"udp://1.1.1.1"}, bad)
	require.Len(t, s.custom, 1)
	require.NoError(t, s.svc.RemoveCustomServer(s.custom[0].ID))
	require.Empty(t, s.custom)
}

func TestService_ListServersMarksInUseAndPinned(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.SetPinned("cf", true))
	require.NoError(t, s.svc.Connect())
	rows := s.svc.ListServers()
	require.Len(t, rows, 1)
	require.True(t, rows[0].InUse)
	require.True(t, rows[0].Pinned)
	require.NotNil(t, rows[0].Result)
	require.True(t, rows[0].Result.OK)
}

func TestService_EmitsStatsEverySecondWhileProtected(t *testing.T) {
	s := newSvc(t)
	ticks := make(chan time.Time)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunStats(s.svc, ctx, ticks)
	ticks <- time.Now()
	require.Never(t, func() bool { return s.em.count(EventStats) > 0 }, 50*time.Millisecond, 5*time.Millisecond)
	require.NoError(t, s.svc.Connect())
	s.eng.mu.Lock()
	s.eng.stats = engine.Stats{Queries: 7, AvgLatency: 24 * time.Millisecond}
	s.eng.mu.Unlock()
	ticks <- time.Now()
	require.Eventually(t, func() bool { return s.em.count(EventStats) == 1 }, time.Second, 5*time.Millisecond)
	s.em.mu.Lock()
	ev := s.em.events[EventStats][0].(StatsEvent)
	s.em.mu.Unlock()
	require.Equal(t, StatsEvent{Queries: 7, LatencyMs: 24}, ev)
}

func TestService_ScanAllEmitsProgress(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.ScanAll())
	require.Eventually(t, func() bool { return s.em.count(EventScan) >= 2 }, time.Second, 5*time.Millisecond)
}

func TestService_BlacklistRoundTrip(t *testing.T) {
	s := newSvc(t)
	txt, err := s.svc.GetDPIBlacklist()
	require.NoError(t, err)
	require.Empty(t, txt)
	require.NoError(t, s.svc.SaveDPIBlacklist("youtube.com\n"))
	txt, _ = s.svc.GetDPIBlacklist()
	require.Equal(t, "youtube.com\n", txt)
}

func TestService_RestoreDNSNowClearsWarning(t *testing.T) {
	s := newSvc(t)
	s.o.AddWarning(AppError{Code: CodeRestoreFailed, Params: map[string]any{"adapter": "Wi-Fi"}})
	require.NoError(t, s.svc.RestoreDNSNow())
	require.Empty(t, s.svc.GetSnapshot().Warnings)
}

func TestService_DismissWarningRefusesRestoreFailed(t *testing.T) {
	s := newSvc(t)
	s.o.AddWarning(AppError{Code: CodeRestoreFailed})
	s.o.AddWarning(AppError{Code: CodeSettingsReset})
	s.svc.DismissWarning(CodeRestoreFailed)
	s.svc.DismissWarning(CodeSettingsReset)
	w := s.svc.GetSnapshot().Warnings
	require.Len(t, w, 1)
	require.Equal(t, CodeRestoreFailed, w[0].Code)
}

func TestBus_StateAndLogEmit(t *testing.T) {
	s := newSvc(t)
	require.NoError(t, s.svc.Connect())
	require.Positive(t, s.em.count(EventState))
	require.Positive(t, s.em.count(EventLog))
	require.NotEmpty(t, s.svc.GetLogs())
}

func TestSaveSettings_RejectsBadTools(t *testing.T) {
	s := newSvc(t)
	st := store.DefaultSettings()
	st.Tools.CFScan.Host = "1.1.1.1"
	require.Error(t, s.svc.SaveSettings(st))
	st = store.DefaultSettings()
	st.Tools.Scanner.Rounds = 21
	require.Error(t, s.svc.SaveSettings(st))
	require.NoError(t, s.svc.SaveSettings(store.DefaultSettings()))
}

func TestSetMode_SimpleAndFull(t *testing.T) {
	s := newSvc(t)
	var got string
	s.svc.x.SetMode = func(m string) { got = m }
	require.NoError(t, s.svc.SetMode(store.ModeFull))
	require.Equal(t, store.ModeFull, s.box.Get().Mode)
	require.Equal(t, store.ModeFull, got)
	require.NoError(t, s.svc.SetMode(store.ModeSimple))
	require.Error(t, s.svc.SetMode("advanced"), "the old name is only read from old files")
}

func TestSaveSettings_KeepsSimpleCustom(t *testing.T) {
	s := newSvc(t)
	st := store.DefaultSettings()
	st.Simple.Custom = &store.SimpleCustom{DPI: true, Proxy: true}
	require.NoError(t, s.svc.SaveSettings(st))
	require.Equal(t, st.Simple, s.box.Get().Simple)
}

// Only the service that holds port 53 may be stopped: the name comes from
// the UI, which any member of the ghostline group can drive.
func TestStopConflictingService_OnlyThePortOwner(t *testing.T) {
	sh := newSvc(t)
	var stopped []string
	sh.svc.x.StopService = func(n string) error { stopped = append(stopped, n); return nil }
	sh.sys.owners = []procs.PortOwner{{PID: 9, Name: "dnsmasq", Service: "dnsmasq.service", Proto: "udp"}}
	err := sh.svc.StopConflictingService("sshd.service")
	require.Equal(t, CodePort53NotOwner, code(t, err))
	require.Empty(t, stopped)
	require.NoError(t, sh.svc.StopConflictingService("dnsmasq.service"))
	require.Equal(t, []string{"dnsmasq.service"}, stopped)
}

// The service buttons are the GUI's own (internal/rpc/client); the daemon
// and Windows have none.
func TestService_InstallActionsAreNotTheDaemons(t *testing.T) {
	s := &Service{}
	require.Equal(t, InstallInfo{}, s.ServiceInstall())
	require.ErrorContains(t, s.InstallService(), CodeServiceActionUnsupported)
	require.ErrorContains(t, s.StartService(), CodeServiceActionUnsupported)
}
