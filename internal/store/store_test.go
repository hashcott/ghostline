package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func TestResolvePaths_PortableWhenMarkerExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "portable"), nil, 0o644))
	p := store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), `C:\AppData`)
	require.True(t, p.Portable)
	require.Equal(t, filepath.Join(dir, "data"), p.DataDir)
	require.Equal(t, filepath.Join(dir, "data", "logs"), p.LogDir)
	require.Equal(t, filepath.Join(dir, "data", "state.json"), p.State)
}

func TestResolvePaths_InstalledUsesAppData(t *testing.T) {
	p := store.ResolvePaths(filepath.Join(t.TempDir(), "ghostline.exe"), `C:\AppData`)
	require.False(t, p.Portable)
	require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline"), p.DataDir)
	require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline", "dpi-blacklist.txt"), p.DPIBlacklist)
}

func TestWriteJSONAtomic_RoundTripLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	require.NoError(t, store.WriteJSONAtomic(path, map[string]int{"a": 1}))
	var got map[string]int
	require.NoError(t, store.ReadJSON(path, &got))
	require.Equal(t, 1, got["a"])
	entries, _ := os.ReadDir(dir)
	require.Len(t, entries, 1)
}

func TestDefaultSettings_MatchSpec(t *testing.T) {
	s := store.DefaultSettings()
	require.Equal(t, 5, s.Version)
	require.Equal(t, "vi", s.Language)
	require.Equal(t, "simple", s.Mode)
	require.True(t, s.CloseToTray)
	require.Equal(t, "auto", s.Adapters)
	require.Equal(t, "www.google.com", s.TestDomain)
	require.Equal(t, []string{"1.1.1.1:53", "8.8.8.8:53"}, s.Bootstrap)
	require.Equal(t, 5, s.MaxUpstreams)
	require.Equal(t, []string{"no-filter"}, s.IncludeTags)
	require.Equal(t, []string{"youtube.com", "discord.com", "x.com"}, s.ProbeSites)
	require.Equal(t, store.DPISettings{Enabled: false, Engine: "zapret2", Preset: "light", CustomArgs: "", Scope: "all", Zapret2: store.Zapret2Settings{Strategy: "z-split"}}, s.DPI)
	require.Equal(t, store.FragmentSettings{Enabled: false, Chunks: 5, DelayMs: 5}, s.FragmentDNS)
	require.Equal(t, store.UpdateSettings{CheckApp: true, UpdateServerList: true}, s.Updates)
	require.Equal(t, store.WindowSize{Width: 1000, Height: 660}, s.FullWindow)
	require.False(t, s.StartWithWindows)
	require.False(t, s.AutoConnect)
	require.False(t, s.PinnedOnly)
	require.Empty(t, s.Pinned)
}

func TestLoadSettings_MissingReturnsDefaults(t *testing.T) {
	s, recovered, err := store.LoadSettings(filepath.Join(t.TempDir(), "settings.json"))
	require.NoError(t, err)
	require.False(t, recovered)
	require.Equal(t, store.DefaultSettings(), s)
}

func TestLoadSettings_PartialFileKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"language":"en"}`), 0o644))
	s, _, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.Equal(t, "en", s.Language)
	require.Equal(t, 5, s.MaxUpstreams)
}

func TestLoadSettings_OldDefaultProbeSitesMoveToNewDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"probeSites":["youtube.com","discord.com","telegram.org","x.com"]}`), 0o644))
	s, _, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.Equal(t, []string{"youtube.com", "discord.com", "x.com"}, s.ProbeSites)
}

func TestLoadSettings_EditedProbeSitesAreKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"probeSites":["telegram.org","example.com"]}`), 0o644))
	s, _, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.Equal(t, []string{"telegram.org", "example.com"}, s.ProbeSites)
}

func TestLoadSettings_CorruptRenamesToBak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte("{oops"), 0o644))
	s, recovered, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.True(t, recovered)
	require.FileExists(t, path+".bak")
	require.Equal(t, store.DefaultSettings(), s)
}

func TestSaveSettings_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := store.DefaultSettings()
	s.Pinned = []string{"cloudflare-doh"}
	require.NoError(t, store.SaveSettings(path, s))
	got, _, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.Equal(t, s, got)
}

type fakeLocker struct{ calls []string }

func (f *fakeLocker) Lock() error   { f.calls = append(f.calls, "lock"); return nil }
func (f *fakeLocker) Unlock() error { f.calls = append(f.calls, "unlock"); return nil }

func TestLoadState_MissingIsClean(t *testing.T) {
	st, err := store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), &fakeLocker{}).Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, st.Phase)
	require.Equal(t, 4, st.Version)
}

func TestLoadState_CorruptReturnsErrStateCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"phase":"dns_`), 0o644))
	_, err := store.NewStateStore(path, &fakeLocker{}).Load()
	require.True(t, errors.Is(err, store.ErrStateCorrupt))
}

func TestStateStore_UpdateLocksAroundReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	l := &fakeLocker{}
	s := store.NewStateStore(path, l)
	require.NoError(t, s.Update(func(st *store.State) error { st.Phase = store.PhaseDNSSet; st.PID = 7; return nil }))
	require.Equal(t, []string{"lock", "unlock"}, l.calls)
	st, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseDNSSet, st.Phase)

	boom := errors.New("boom")
	require.ErrorIs(t, s.Update(func(st *store.State) error { st.Phase = store.PhaseClean; return boom }), boom)
	require.Equal(t, []string{"lock", "unlock", "lock", "unlock"}, l.calls)
	st, _ = s.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase, "failed update must not be written")
}

func TestStateStore_UpdateOnCorruptPassesErr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"phase":"dns_`), 0o644))
	err := store.NewStateStore(path, &fakeLocker{}).Update(func(*store.State) error { return nil })
	require.ErrorIs(t, err, store.ErrStateCorrupt)
}

func TestStateStore_Reset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"phase":"dns_`), 0o644))
	s := store.NewStateStore(path, &fakeLocker{})
	require.NoError(t, s.Reset())
	st, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, st.Phase)
}

func TestMeta_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.json")
	require.Equal(t, store.Meta{}, store.LoadMeta(path))
	m := store.Meta{LastUpdateCheck: time.Date(2026, 10, 4, 1, 2, 3, 0, time.UTC)}
	require.NoError(t, store.SaveMeta(path, m))
	require.True(t, store.LoadMeta(path).LastUpdateCheck.Equal(m.LastUpdateCheck))
}

func TestLoadSettings_AcceptsUTF8BOM(t *testing.T) { // Notepad may add a BOM
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"mode":"advanced"}`)...), 0o644))
	s, recovered, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Equal(t, store.ModeFull, s.Mode, "\"advanced\" from older files becomes \"full\"")
}
