package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

// v0.5 reads "snapshot" (adapters) and a flat WinINET "sysproxy.snapshot".
// A crash, then a downgrade, must still find what to restore.
type v05State struct {
	Snapshot []model.AdapterSnapshot `json:"snapshot"`
	SysProxy *struct {
		Snapshot *struct {
			Flags         uint32 `json:"flags"`
			Server        string `json:"server"`
			Bypass        string `json:"bypass"`
			AutoconfigURL string `json:"autoconfigUrl"`
		} `json:"snapshot"`
	} `json:"sysproxy"`
}

func TestWrite_WindowsStateReadableByV05(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := store.NewStateStore(path, &fakeLocker{})
	adapters := []model.AdapterSnapshot{{GUID: "{A}", Alias: "Wi-Fi"}}
	require.NoError(t, s.Update(func(st *store.State) error {
		*st = store.CleanState()
		st.Phase = store.PhaseDNSSet
		st.DNS = model.DNSSnapshot{Backend: "windows", Windows: adapters}
		st.SysProxy = &store.SysProxyState{Ours: "127.0.0.1:8080", Snapshot: &model.ProxySnapshot{Backend: "windows", Windows: &model.WinINETProxy{Flags: 5, AutoconfigURL: "http://pac"}}}
		return nil
	}))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var old v05State
	require.NoError(t, json.Unmarshal(b, &old))
	require.Equal(t, adapters, old.Snapshot)
	require.Equal(t, uint32(5), old.SysProxy.Snapshot.Flags)
	require.Equal(t, "http://pac", old.SysProxy.Snapshot.AutoconfigURL)

	got, err := s.Load() // and the current build still reads its own form
	require.NoError(t, err)
	require.Equal(t, adapters, got.DNS.Windows)
	require.Equal(t, "windows", got.SysProxy.Snapshot.Backend)
	require.Equal(t, uint32(5), got.SysProxy.Snapshot.Windows.Flags)
}

// Linux snapshots carry no legacy fields.
func TestWrite_LinuxStateHasNoLegacyFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := store.NewStateStore(path, &fakeLocker{})
	require.NoError(t, s.Update(func(st *store.State) error {
		*st = store.CleanState()
		st.DNS = model.DNSSnapshot{Backend: "resolved", Linux: &model.LinuxDNS{}}
		st.SysProxy = &store.SysProxyState{Snapshot: &model.ProxySnapshot{Backend: "kde", KDE: &model.KDEProxy{}}}
		return nil
	}))
	b, _ := os.ReadFile(path)
	require.NotContains(t, string(b), `"snapshot": [`)
	require.NotContains(t, string(b), `"flags"`)
}
