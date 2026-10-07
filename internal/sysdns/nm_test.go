package sysdns

import (
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/stretchr/testify/require"
)

// fakeNM is NetworkManager: DNSServers follows the global configuration
// unless stuck is set (NM did not apply it).
type fakeNM struct {
	global model.NMGlobalDNS
	isp    []string // servers NM uses without a global configuration
	stuck  bool
	sets   int
}

func (f *fakeNM) Running() bool                         { return true }
func (f *fakeNM) DNSMode() (string, error)              { return "systemd-resolved", nil }
func (f *fakeNM) RcManager() (string, error)            { return "symlink", nil }
func (f *fakeNM) GlobalDNS() (model.NMGlobalDNS, error) { return f.global, nil }
func (f *fakeNM) SetGlobalDNS(g model.NMGlobalDNS) error {
	f.sets++
	if !f.stuck {
		f.global = g
	}
	return nil
}
func (f *fakeNM) DNSServers() ([]string, error) {
	if d, ok := f.global.Domains["*"]; ok && len(d.Servers) > 0 {
		return d.Servers, nil
	}
	return f.isp, nil
}
func (f *fakeNM) Watch(func()) (func(), error) { return func() {}, nil }

func newTestNM(f *fakeNM) Backend {
	b := newNM(f, func() error { return nil }, nil, "NetworkManager → systemd-resolved").(*nmBackend)
	b.sleep = func(time.Duration) {}
	return b
}

func TestNM_ApplyRestoreRoundTrip(t *testing.T) {
	f := &fakeNM{global: model.NMGlobalDNS{Searches: []string{"lan"}}, isp: []string{"192.168.1.1"}}
	b := newTestNM(f)
	require.Equal(t, "networkmanager", b.Name())
	s, err := b.Snapshot(Selection{})
	require.NoError(t, err)
	require.Equal(t, []string{"192.168.1.1"}, s.Servers())
	require.NoError(t, b.Apply(s, true))
	require.Equal(t, []string{"127.0.0.1", "::1"}, f.global.Domains["*"].Servers)
	require.Equal(t, []string{"lan"}, f.global.Searches, "searches kept")
	require.Empty(t, b.Restore(s))
	require.Equal(t, model.NMGlobalDNS{Searches: []string{"lan"}}, f.global)
}

func TestNM_ApplyFailsWhenNMDoesNotFollow(t *testing.T) {
	f := &fakeNM{isp: []string{"192.168.1.1"}, stuck: true}
	b := newTestNM(f)
	s, err := b.Snapshot(Selection{})
	require.NoError(t, err)
	require.ErrorContains(t, b.Apply(s, false), "did not change")
}

// Review Focus 3: someone edits NM's global DNS while connected.
func TestNM_ReconcileReappliesAndKeepsOriginal(t *testing.T) {
	f := &fakeNM{isp: []string{"192.168.1.1"}}
	b := newTestNM(f)
	s, err := b.Snapshot(Selection{})
	require.NoError(t, err)
	require.NoError(t, b.Apply(s, false))

	next, toApply, changes, err := b.Reconcile(s, Selection{})
	require.NoError(t, err)
	require.Equal(t, s, next)
	require.Empty(t, changes, "still ours: nothing to do")
	require.True(t, toApply.Empty())

	f.global = model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"9.9.9.9"}}}}
	next, toApply, changes, err = b.Reconcile(s, Selection{})
	require.NoError(t, err)
	require.Equal(t, []Change{{Target: "NetworkManager"}}, changes)
	require.Equal(t, s, next, "the original from before Connect is kept")
	require.NoError(t, b.Apply(toApply, false))
	require.Equal(t, []string{"127.0.0.1"}, f.global.Domains["*"].Servers)
	require.Empty(t, b.Restore(next))
	require.Equal(t, model.NMGlobalDNS{}, f.global, "Disconnect restores the value from before Connect")
}

// Review Focus 5: a crash before Apply leaves NM untouched; recovery must
// not overwrite whatever is there.
func TestNM_StillOursOnlyWhenLoopback(t *testing.T) {
	f := &fakeNM{global: model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"9.9.9.9"}}}}}
	b := newTestNM(f)
	s := model.DNSSnapshot{Backend: "networkmanager", Linux: &model.LinuxDNS{NMGlobal: &model.NMGlobalDNS{}}}
	require.True(t, b.StillOurs(s).Empty())
	f.global = model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"127.0.0.1", "::1"}}}}
	require.Equal(t, s, b.StillOurs(s))
}

func TestNM_RestoreDefaultClearsOnlyOurs(t *testing.T) {
	f := &fakeNM{global: model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"127.0.0.1"}}}}}
	require.NoError(t, newTestNM(f).RestoreDefault())
	require.Equal(t, model.NMGlobalDNS{}, f.global)

	other := model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"9.9.9.9"}}}}
	f = &fakeNM{global: other}
	require.NoError(t, newTestNM(f).RestoreDefault())
	require.Equal(t, other, f.global)
	require.Zero(t, f.sets)
}

// Review I4: Connect over Ghostline's own leftover (state.json lost) must
// not record loopback as the original.
func TestNM_SnapshotNeverRecordsLoopback(t *testing.T) {
	f := &fakeNM{global: model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"127.0.0.1"}}}}}
	s, err := newTestNM(f).Snapshot(Selection{})
	require.NoError(t, err)
	require.Equal(t, &model.NMGlobalDNS{}, s.Linux.NMGlobal)
}
