package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

// A v0.6-L3 (v4) run that crashed with the system proxy set left a WinINET
// snapshot; it becomes the "windows" branch of the neutral snapshot.
func TestLoad_V4ProxySnapshotMigrates(t *testing.T) {
	st, err := loadState(t, `{"version":4,"phase":"dns_set","pid":7,"sysproxy":{"set":true,"ours":"127.0.0.1:8080","snapshot":{"flags":5,"server":"","bypass":"","autoconfigUrl":"http://pac"}}}`)
	require.NoError(t, err)
	require.Equal(t, 5, st.Version)
	require.Equal(t, &model.ProxySnapshot{Backend: "windows", Windows: &model.WinINETProxy{Flags: 5, AutoconfigURL: "http://pac"}}, st.SysProxy.Snapshot)
	require.True(t, st.SysProxy.Set)
}

func TestWrite_V5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := store.NewStateStore(path, &fakeLocker{})
	require.NoError(t, s.Update(func(st *store.State) error {
		*st = store.CleanState()
		st.SysProxy = &store.SysProxyState{Ours: "127.0.0.1:8080", Snapshot: &model.ProxySnapshot{Backend: "kde", UID: 1000, KDE: &model.KDEProxy{Values: map[string]string{"ProxyType": "0"}, Present: []string{"ProxyType"}}}}
		return nil
	}))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(b), `"version": 5`)
	require.Contains(t, string(b), `"backend": "kde"`)
	got, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, 1000, got.SysProxy.Snapshot.UID)
	require.Equal(t, "0", got.SysProxy.Snapshot.KDE.Values["ProxyType"])
}
