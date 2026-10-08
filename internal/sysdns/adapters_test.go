package sysdns_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

func wifi(guid string, idx uint32) sysdns.Adapter {
	return sysdns.Adapter{GUID: guid, IfIndex: idx, Alias: "Wi-Fi", IfType: 71, Up: true, HasGateway: true}
}

func noWatch(func()) (func(), error) { return func() {}, nil }

func TestAdapterBackend_SnapshotApplyRestore(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{wifi("{A}", 3)}, dns: map[string][]string{}}
	b := sysdns.NewAdapterBackend(api, noWatch)
	require.Equal(t, "windows", b.Name())
	s, err := b.Snapshot(sysdns.Selection{Mode: "auto"})
	require.NoError(t, err)
	require.Equal(t, "windows", s.Backend)
	require.Len(t, s.Windows, 1)
	require.NoError(t, b.Apply(s, false))
	require.Equal(t, []string{"127.0.0.1"}, api.dns["{A}|v4"])
	require.Empty(t, b.Restore(s))
	require.Empty(t, api.dns["{A}|v4"], "back to DHCP")

	_, err = sysdns.NewAdapterBackend(&fakeAPI{dns: map[string][]string{}}, noWatch).Snapshot(sysdns.Selection{Mode: "auto"})
	require.Error(t, err, "no connected adapters")
}

// Review Focus 1: an adapter that appears while connected is recorded
// before it is changed, and reported as added.
func TestAdapterBackend_ReconcileAddsNewAdapter(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{wifi("{A}", 3)}, dns: map[string][]string{}}
	b := sysdns.NewAdapterBackend(api, noWatch)
	s, err := b.Snapshot(sysdns.Selection{Mode: "auto"})
	require.NoError(t, err)
	require.NoError(t, b.Apply(s, false))

	nb := eth("{B}", 7)
	nb.Alias = "Ethernet 2"
	api.adapters = append(api.adapters, nb)
	api.dns["{B}|v4"] = []string{"8.8.8.8"}
	next, toApply, changes, err := b.Reconcile(s, sysdns.Selection{Mode: "auto"})
	require.NoError(t, err)
	require.Len(t, next.Windows, 2)
	require.Equal(t, []string{"8.8.8.8"}, next.Windows[1].IPv4.Servers, "recorded as it was")
	require.Equal(t, []sysdns.Change{{Target: "Ethernet 2", Added: true}}, changes)
	require.Equal(t, []string{"8.8.8.8"}, api.dns["{B}|v4"], "Reconcile changes nothing")
	require.Len(t, toApply.Windows, 1, "only the new adapter is applied")
	require.Equal(t, "{B}", toApply.Windows[0].GUID)
	require.NoError(t, b.Apply(toApply, false))
	require.Equal(t, []string{"127.0.0.1"}, api.dns["{B}|v4"])

	next2, toApply2, changes, err := b.Reconcile(next, sysdns.Selection{Mode: "auto"})
	require.NoError(t, err)
	require.Empty(t, changes, "nothing new")
	require.True(t, toApply2.Empty())
	require.Equal(t, next, next2)
}

// Review Focus 2: an adapter the user re-configured after a crash keeps
// the user's settings.
func TestAdapterBackend_StillOursKeepsUserChanges(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{wifi("{A}", 3), eth("{B}", 7)}, dns: map[string][]string{
		"{A}|v4": {"127.0.0.1"}, "{B}|v4": {"9.9.9.9"},
	}}
	b := sysdns.NewAdapterBackend(api, noWatch)
	s := model.DNSSnapshot{Backend: "windows", Windows: []model.AdapterSnapshot{{GUID: "{A}", Alias: "Wi-Fi"}, {GUID: "{B}", Alias: "Ethernet"}}}
	got := b.StillOurs(s)
	require.Len(t, got.Windows, 1)
	require.Equal(t, "{A}", got.Windows[0].GUID)
}

func TestAdapterBackend_RestoreDefaultResetsLoopbackOnly(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{wifi("{A}", 3), eth("{B}", 7)}, dns: map[string][]string{
		"{A}|v4": {"127.0.0.1"}, "{B}|v4": {"8.8.8.8"},
	}}
	b := sysdns.NewAdapterBackend(api, noWatch)
	require.NoError(t, b.RestoreDefault())
	require.Empty(t, api.dns["{A}|v4"])
	require.Equal(t, []string{"8.8.8.8"}, api.dns["{B}|v4"])
	info := b.Info()
	require.True(t, info.AdapterPick)
	require.Len(t, info.Adapters, 2)
}

func TestUnsupportedBackend(t *testing.T) {
	var b sysdns.Backend = sysdns.UnsupportedBackend{}
	_, err := b.Snapshot(sysdns.Selection{})
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, b.Apply(model.DNSSnapshot{}, false), errors.ErrUnsupported)
	require.ErrorIs(t, b.RestoreDefault(), errors.ErrUnsupported)
	require.Empty(t, b.Restore(model.DNSSnapshot{}))
	require.True(t, b.StillOurs(model.DNSSnapshot{Backend: "x"}).Empty())
	require.NoError(t, b.Flush())
	stop, err := b.Watch(func() {})
	require.NoError(t, err)
	stop()
	require.Equal(t, "unsupported", b.Info().Backend)
}

// If the current DNS cannot be read, restoring everything is the safe side.
func TestAdapterBackend_StillOursKeepsAllWhenUnreadable(t *testing.T) {
	api := &fakeAPI{adaptersErr: errors.New("access denied"), dns: map[string][]string{}}
	b := sysdns.NewAdapterBackend(api, noWatch)
	s := model.DNSSnapshot{Backend: "windows", Windows: []model.AdapterSnapshot{{GUID: "{A}"}, {GUID: "{B}"}}}
	require.Equal(t, s, b.StillOurs(s))
}
