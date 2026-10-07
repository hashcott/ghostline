package watchdog_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/stretchr/testify/require"
)

type memLock struct{ mu sync.Mutex }

func (l *memLock) Lock() error   { l.mu.Lock(); return nil }
func (l *memLock) Unlock() error { l.mu.Unlock(); return nil }

type fakeDNS struct {
	restored []model.AdapterSnapshot
	loopback []sysdns.Adapter
}

func (f *fakeDNS) Restore(s sysdns.Snapshot) []sysdns.RestoreError {
	f.restored = append(f.restored, s.Windows...)
	return nil
}

// StillOurs keeps the adapters still on loopback, as the Windows backend does.
func (f *fakeDNS) StillOurs(s sysdns.Snapshot) sysdns.Snapshot {
	on := map[string]bool{}
	for _, a := range f.loopback {
		on[a.GUID] = true
	}
	out := sysdns.Snapshot{Backend: s.Backend}
	for _, a := range s.Windows {
		if on[a.GUID] {
			out.Windows = append(out.Windows, a)
		}
	}
	return out
}

// RestoreDefault resets loopback adapters to DHCP, as the Windows backend does.
func (f *fakeDNS) RestoreDefault() error {
	for _, a := range f.loopback {
		f.restored = append(f.restored, model.AdapterSnapshot{GUID: a.GUID, Alias: a.Alias,
			IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}, IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}})
	}
	return nil
}

var snap = model.AdapterSnapshot{GUID: "{A}", IfIndex: 3, Alias: "Wi-Fi",
	IPv4: model.FamilyDNS{Mode: model.DNSModeStatic, Servers: []string{"9.9.9.9"}}, IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}}

func setup(t *testing.T, alive bool, st *store.State) (watchdog.Deps, *fakeDNS, *int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	ss := store.NewStateStore(path, &memLock{})
	if st != nil {
		require.NoError(t, ss.Update(func(s *store.State) error { *s = *st; return nil }))
	}
	fd := &fakeDNS{loopback: []sysdns.Adapter{{GUID: snap.GUID}}} // still pointing at Ghostline
	stops := 0
	return watchdog.Deps{
		States: ss, DNS: fd,
		StopDPI: func() error { stops++; return nil },
		Alive:   func(uint32, time.Time) bool { return alive },
	}, fd, &stops
}

func dirty() *store.State {
	return &store.State{Version: 1, Phase: store.PhaseDNSSet, PID: 42, PIDStartTime: time.Unix(100, 0),
		DNS: model.DNSSnapshot{Backend: "windows", Windows: []model.AdapterSnapshot{snap}}, DPI: store.DPIState{Running: true, PID: 7}}
}

func TestRestore_CleanDoesNothing(t *testing.T) {
	d, fd, _ := setup(t, false, nil)
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.NothingToDo, out)
	require.Empty(t, fd.restored)
}

func TestRestore_OwnerAliveDoesNothing(t *testing.T) {
	d, fd, _ := setup(t, true, dirty())
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.OwnerAlive, out)
	require.Empty(t, fd.restored)
}

func TestRestore_RestoresSnapshotStopsDPIMarksClean(t *testing.T) {
	d, fd, stops := setup(t, false, dirty())
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.Restored, out)
	require.Equal(t, []model.AdapterSnapshot{snap}, fd.restored)
	require.Equal(t, 1, *stops)
	st, _ := d.States.Load()
	require.Equal(t, store.PhaseClean, st.Phase)
	require.Empty(t, st.DNS.Windows)
}

func TestRestore_PIDReusedStillRestores(t *testing.T) {
	d, fd, _ := setup(t, false, dirty())
	var gotStart time.Time
	d.Alive = func(pid uint32, start time.Time) bool { gotStart = start; return false }
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.Restored, out)
	require.True(t, gotStart.Equal(time.Unix(100, 0)), "liveness must be checked with the recorded start time")
	require.Len(t, fd.restored, 1)
}

func TestRestore_CorruptStateFallsBackToLoopbackAdapters(t *testing.T) { // Review Focus #2
	d, fd, stops := setup(t, false, nil)
	fd.loopback = []sysdns.Adapter{{GUID: "{B}", IfIndex: 9, Alias: "Ethernet"}}
	path := filepath.Join(filepath.Dir(statePath(t, d)), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"phase":"dns_`), 0o644))
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.RestoredFromCorrupt, out)
	require.Len(t, fd.restored, 1)
	require.Equal(t, "{B}", fd.restored[0].GUID)
	require.Equal(t, model.DNSModeDHCP, fd.restored[0].IPv4.Mode)
	require.Equal(t, model.DNSModeDHCP, fd.restored[0].IPv6.Mode)
	require.Equal(t, 1, *stops)
	st, err := d.States.Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, st.Phase)
}

// statePath recovers the state file path by writing through the store.
func statePath(t *testing.T, d watchdog.Deps) string {
	t.Helper()
	require.NoError(t, d.States.Reset())
	return d.States.Path()
}

func TestRunWatchdog_RestoresAfterParentExit(t *testing.T) {
	d, fd, _ := setup(t, false, dirty())
	waited := uint32(0)
	require.NoError(t, watchdog.RunWatchdog(42, time.Unix(100, 0), func(pid uint32) error { waited = pid; return nil }, d))
	require.Equal(t, uint32(42), waited)
	require.Len(t, fd.restored, 1)
}

type failingDNS struct{ fakeDNS }

func (f *failingDNS) Restore(s sysdns.Snapshot) []sysdns.RestoreError {
	return []sysdns.RestoreError{{Target: s.Label(), Err: os.ErrPermission}}
}

func TestRestore_FailureKeepsSnapshotForLaterLayers(t *testing.T) {
	d, _, _ := setup(t, false, dirty())
	d.DNS = &failingDNS{fakeDNS{loopback: []sysdns.Adapter{{GUID: snap.GUID}}}}
	out, err := watchdog.RestoreIfOrphaned(d)
	require.Error(t, err)
	require.Equal(t, watchdog.Restored, out)
	st, _ := d.States.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase)
	require.Len(t, st.DNS.Windows, 1)
}

func TestRestore_CorruptStateWithFailedResetStaysUnclean(t *testing.T) {
	d, _, _ := setup(t, false, nil)
	d.DNS = &failingLoopback{}
	require.NoError(t, os.WriteFile(d.States.Path(), []byte(`{"phase":"dns_`), 0o644))
	_, err := watchdog.RestoreIfOrphaned(d)
	require.Error(t, err)
	_, lerr := d.States.Load()
	require.Error(t, lerr, "a failed reset must not be recorded as clean")
}

type failingLoopback struct{}

func (failingLoopback) Restore(sysdns.Snapshot) []sysdns.RestoreError {
	return []sysdns.RestoreError{{Target: "{B}", Err: os.ErrPermission}}
}
func (failingLoopback) StillOurs(s sysdns.Snapshot) sysdns.Snapshot { return s }
func (failingLoopback) RestoreDefault() error                       { return os.ErrPermission }

func TestRunWatchdog_KeepsWaitingWhileParentAlive(t *testing.T) { // review minor
	d, fd, _ := setup(t, false, dirty())
	alive := 2
	d.Alive = func(uint32, time.Time) bool { alive--; return alive >= 0 }
	d.Sleep = func(time.Duration) {}
	waits := 0
	require.NoError(t, watchdog.RunWatchdog(42, time.Unix(100, 0), func(uint32) error { waits++; return nil }, d))
	require.Equal(t, 3, waits, "a spurious wakeup must not end the watch")
	require.Len(t, fd.restored, 1)
}

type brokenLookup struct{ fakeDNS }

// StillOurs cannot read the current DNS: the backend keeps everything.
func (*brokenLookup) StillOurs(s sysdns.Snapshot) sysdns.Snapshot { return s }

// An adapter the user re-configured by hand after a crash keeps their DNS;
// only adapters still pointing at Ghostline's loopback are restored.
func TestRestore_SkipsAdaptersNoLongerOnLoopback(t *testing.T) {
	other := model.AdapterSnapshot{GUID: "{B}", Alias: "Ethernet", IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}}
	st := dirty()
	st.DNS.Windows = append(st.DNS.Windows, other)
	d, fd, _ := setup(t, false, st)
	fd.loopback = []sysdns.Adapter{{GUID: "{B}"}} // {A} was changed by the user
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.Restored, out)
	require.Equal(t, []model.AdapterSnapshot{other}, fd.restored)
	st2, _ := d.States.Load()
	require.Equal(t, store.PhaseClean, st2.Phase)
}

// If the current DNS cannot be read, restoring everything is the safe side.
func TestRestore_LoopbackLookupFailureRestoresAll(t *testing.T) {
	d, _, _ := setup(t, false, dirty())
	bl := &brokenLookup{}
	d.DNS = bl
	_, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, []model.AdapterSnapshot{snap}, bl.restored)
}

// The restore uses the backend's StillOurs on the recorded snapshot.
func TestRestore_OrphanedRestoresOnlyStillOurs(t *testing.T) {
	other := model.AdapterSnapshot{GUID: "{B}", Alias: "Ethernet"}
	st := dirty()
	st.DNS.Windows = append(st.DNS.Windows, other)
	d, fd, _ := setup(t, false, st)
	fd.loopback = []sysdns.Adapter{{GUID: snap.GUID}}
	_, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, []model.AdapterSnapshot{snap}, fd.restored)
}

type defaultOnly struct {
	fakeDNS
	defaults int
}

func (d *defaultOnly) RestoreDefault() error { d.defaults++; return nil }

func TestRestore_CorruptCallsRestoreDefault(t *testing.T) {
	d, _, _ := setup(t, false, nil)
	do := &defaultOnly{}
	d.DNS = do
	require.NoError(t, os.WriteFile(d.States.Path(), []byte("{bad"), 0o644))
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.RestoredFromCorrupt, out)
	require.Equal(t, 1, do.defaults)
	require.Empty(t, do.restored)
}
