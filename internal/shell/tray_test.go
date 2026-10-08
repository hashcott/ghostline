package shell

import (
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type fakeTray struct {
	status  app.Status
	clients int
	calls   []string
}

func (f *fakeTray) GetSnapshot() app.Snapshot                { return app.Snapshot{Status: f.status} }
func (f *fakeTray) GetSettings() store.Settings              { return store.DefaultSettings() }
func (f *fakeTray) SaveSettings(store.Settings) error        { f.calls = append(f.calls, "save"); return nil }
func (f *fakeTray) Connect() error                           { f.calls = append(f.calls, "connect"); return nil }
func (f *fakeTray) Disconnect() error                        { f.calls = append(f.calls, "disconnect"); return nil }
func (f *fakeTray) SetDPIEnabled(bool) error                 { return nil }
func (f *fakeTray) SetProxyEnabled(bool) error               { return nil }
func (f *fakeTray) CheckUpdateNow() (app.UpdateCheck, error) { return app.UpdateCheck{}, nil }
func (f *fakeTray) LANDNSClients() int                       { return f.clients }

func TestTrayToggle_ConnectsWhenDisconnected(t *testing.T) {
	f := &fakeTray{status: app.StatusDisconnected}
	asked := false
	require.NoError(t, trayToggle(f, func(int) bool { asked = true; return true }))
	require.Equal(t, []string{"connect"}, f.calls)
	require.False(t, asked)
}

func TestTrayToggle_AsksBeforeDisconnectWithLANClients(t *testing.T) {
	f := &fakeTray{status: app.StatusProtected, clients: 3}
	var seen int
	require.NoError(t, trayToggle(f, func(n int) bool { seen = n; return false }))
	require.Equal(t, 3, seen)
	require.Empty(t, f.calls)
	require.NoError(t, trayToggle(f, func(int) bool { return true }))
	require.Equal(t, []string{"disconnect"}, f.calls)
}

func TestTrayToggle_DisconnectsWithoutAskingWhenNoClients(t *testing.T) {
	f := &fakeTray{status: app.StatusDegraded}
	asked := false
	require.NoError(t, trayToggle(f, func(int) bool { asked = true; return true }))
	require.Equal(t, []string{"disconnect"}, f.calls)
	require.False(t, asked)
}
