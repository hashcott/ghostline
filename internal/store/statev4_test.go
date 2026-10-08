package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func loadState(t *testing.T, body string) (store.State, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return store.NewStateStore(path, &fakeLocker{}).Load()
}

func TestLoad_V3CleanBecomesV4(t *testing.T) {
	st, err := loadState(t, `{"version":3,"phase":"clean","snapshot":null}`)
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, st.Phase)
	require.True(t, st.DNS.Empty())
}

// A v0.5 run that crashed while connected left its adapters in "snapshot".
func TestLoad_V3DirtyMigratesAdapters(t *testing.T) {
	st, err := loadState(t, `{"version":3,"phase":"dns_set","pid":7,"snapshot":[{"guid":"{A}","alias":"Wi-Fi","ipv4":{"mode":"static","servers":["1.1.1.1"]},"ipv6":{"mode":"dhcp"}}]}`)
	require.NoError(t, err)
	require.Equal(t, store.PhaseDNSSet, st.Phase)
	require.Equal(t, uint32(7), st.PID)
	require.Equal(t, "windows", st.DNS.Backend)
	require.Len(t, st.DNS.Windows, 1)
	require.Equal(t, "{A}", st.DNS.Windows[0].GUID)
	require.Equal(t, []string{"1.1.1.1"}, st.DNS.Windows[0].IPv4.Servers)
}

func TestLoad_V3CorruptStillCorrupt(t *testing.T) {
	_, err := loadState(t, "{bad")
	require.ErrorIs(t, err, store.ErrStateCorrupt)
}

func TestWrite_V4HasDNSNotSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := store.NewStateStore(path, &fakeLocker{})
	require.NoError(t, s.Update(func(st *store.State) error {
		st.Phase = store.PhaseDNSSet
		st.DNS = model.DNSSnapshot{Backend: "networkmanager", Linux: &model.LinuxDNS{NMGlobal: &model.NMGlobalDNS{}}}
		return nil
	}))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(b), `"dns"`)
	require.Contains(t, string(b), `"version": 5`)
	require.NotContains(t, string(b), `"snapshot":`)
	st, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, "networkmanager", st.DNS.Backend)
	require.Equal(t, 5, store.CleanState().Version)
}
