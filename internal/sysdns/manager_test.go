package sysdns_test

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	adapters    []sysdns.Adapter
	adaptersErr error
	dns         map[string][]string // guid|v4 or guid|v6
	setErr      error
	netshErr    error
	netshDHCP   error
	netshCalls  []string
	setCalls    int
	flushes     int
}

func key(guid string, v6 bool) string {
	if v6 {
		return guid + "|v6"
	}
	return guid + "|v4"
}

func (f *fakeAPI) Adapters() ([]sysdns.Adapter, error) { return f.adapters, f.adaptersErr }
func (f *fakeAPI) GetDNS(guid string, v6 bool) ([]string, error) {
	return f.dns[key(guid, v6)], nil
}
func (f *fakeAPI) SetDNS(guid string, v6 bool, servers []string) error {
	f.setCalls++
	if f.setErr != nil {
		return f.setErr
	}
	f.dns[key(guid, v6)] = servers
	return nil
}
func (f *fakeAPI) NetshSetDNS(ifIndex uint32, v6 bool, servers []string) error {
	fam := "v4"
	if v6 {
		fam = "v6"
	}
	f.netshCalls = append(f.netshCalls, fmt.Sprintf("netsh:%d:%s:%s", ifIndex, fam, joined(servers)))
	if len(servers) == 0 && f.netshDHCP != nil {
		return f.netshDHCP
	}
	if len(servers) > 0 && f.netshErr != nil {
		return f.netshErr
	}
	for _, a := range f.adapters {
		if a.IfIndex == ifIndex {
			f.dns[key(a.GUID, v6)] = servers
		}
	}
	return nil
}
func (f *fakeAPI) Flush() error { f.flushes++; return nil }

func joined(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}

func eth(guid string, idx uint32) sysdns.Adapter {
	return sysdns.Adapter{GUID: guid, IfIndex: idx, Alias: "Ethernet", IfType: 6, Up: true, HasGateway: true, HasIPv6: true}
}

func newMgr(api *fakeAPI) (*sysdns.Manager, *atomic.Int32) {
	sleeps := &atomic.Int32{}
	return sysdns.NewManager(api, func(time.Duration) { sleeps.Add(1) }), sleeps
}

func TestSelectAuto(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{
		eth("{A}", 1),
		{GUID: "{B}", IfType: 71, Up: false, HasGateway: true},
		{GUID: "{C}", IfType: 24, Up: true, HasGateway: true},
		{GUID: "{D}", IfType: 6, Up: true, HasGateway: false},
		{GUID: "{E}", IfType: 71, Up: true, HasGateway: true},
	}, dns: map[string][]string{}}
	m, _ := newMgr(api)
	got, err := m.Select("auto", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"{A}", "{E}"}, []string{got[0].GUID, got[1].GUID})

	got, err = m.Select("manual", []string{"{D}", "{gone}"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "{D}", got[0].GUID)
}

func TestSnapshotApplyRestore_RoundTrip(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{eth("{A}", 12)}, dns: map[string][]string{"{A}|v6": {"2001:db8::1"}}}
	m, _ := newMgr(api)
	ads, _ := m.Select("auto", nil)
	snaps, err := m.Snapshot(ads)
	require.NoError(t, err)
	require.Equal(t, model.FamilyDNS{Mode: model.DNSModeDHCP}, snaps[0].IPv4)
	require.Equal(t, model.FamilyDNS{Mode: model.DNSModeStatic, Servers: []string{"2001:db8::1"}}, snaps[0].IPv6)
	require.Equal(t, uint32(12), snaps[0].IfIndex)

	require.NoError(t, m.ApplyLoopback(snaps, true))
	require.Equal(t, []string{"127.0.0.1"}, api.dns["{A}|v4"])
	require.Equal(t, []string{"::1"}, api.dns["{A}|v6"])

	require.Empty(t, m.Restore(snaps))
	require.Empty(t, api.dns["{A}|v4"])
	require.Equal(t, []string{"2001:db8::1"}, api.dns["{A}|v6"])
	require.GreaterOrEqual(t, api.flushes, 1)
}

func TestApplyLoopback_SkipsV6WhenDisabled(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{eth("{A}", 1)}, dns: map[string][]string{}}
	m, _ := newMgr(api)
	ads, _ := m.Select("auto", nil)
	snaps, _ := m.Snapshot(ads)
	require.NoError(t, m.ApplyLoopback(snaps, false))
	require.Empty(t, api.dns["{A}|v6"])
}

func TestRestore_KeysByGUIDAndNetshUsesIfIndex(t *testing.T) { // Review Focus #3
	api := &fakeAPI{adapters: []sysdns.Adapter{{GUID: "{A}", IfIndex: 12, Alias: "Ethernet 2", IfType: 6, Up: true, HasGateway: true}},
		dns: map[string][]string{}, setErr: errors.New("api down")}
	m, _ := newMgr(api)
	snaps := []model.AdapterSnapshot{{GUID: "{A}", IfIndex: 12, Alias: "Kết nối mạng cục bộ",
		IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}}}
	errs := m.Restore(snaps)
	require.Empty(t, errs)
	require.Equal(t, []string{"netsh:12:v4:"}, api.netshCalls)
}

func TestRestore_RetriesThenFallsBackThenReports(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{eth("{A}", 7)}, dns: map[string][]string{},
		setErr: errors.New("api"), netshErr: errors.New("netsh"), netshDHCP: errors.New("dhcp")}
	m, sleeps := newMgr(api)
	snaps := []model.AdapterSnapshot{{GUID: "{A}", IfIndex: 7, Alias: "Ethernet",
		IPv4: model.FamilyDNS{Mode: model.DNSModeStatic, Servers: []string{"9.9.9.9"}}}}
	errs := m.Restore(snaps)
	require.Len(t, errs, 1)
	require.Equal(t, "Ethernet", errs[0].Target)
	require.Equal(t, 3, api.setCalls)
	require.Equal(t, int32(2), sleeps.Load())
	require.Equal(t, []string{"netsh:7:v4:9.9.9.9", "netsh:7:v4:"}, api.netshCalls)
}

func TestRestore_MissingAdapterSkipped(t *testing.T) {
	api := &fakeAPI{dns: map[string][]string{}}
	m, _ := newMgr(api)
	require.Empty(t, m.Restore([]model.AdapterSnapshot{{GUID: "{gone}", IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}}}))
	require.Zero(t, api.setCalls)
}

func TestRestore_Idempotent(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{eth("{A}", 1)}, dns: map[string][]string{"{A}|v4": {"127.0.0.1"}}}
	m, _ := newMgr(api)
	snaps := []model.AdapterSnapshot{{GUID: "{A}", IfIndex: 1, IPv4: model.FamilyDNS{Mode: model.DNSModeStatic, Servers: []string{"9.9.9.9"}}}}
	require.Empty(t, m.Restore(snaps))
	require.Empty(t, m.Restore(snaps))
	require.Equal(t, []string{"9.9.9.9"}, api.dns["{A}|v4"])
}

func TestLoopbackAdapters(t *testing.T) {
	api := &fakeAPI{adapters: []sysdns.Adapter{eth("{A}", 1), eth("{B}", 2), eth("{C}", 3)},
		dns: map[string][]string{"{A}|v4": {"127.0.0.1"}, "{B}|v6": {"::1"}, "{C}|v4": {"127.0.0.1", "8.8.8.8"}}}
	m, _ := newMgr(api)
	got, err := m.LoopbackAdapters()
	require.NoError(t, err)
	require.Equal(t, []string{"{A}", "{B}"}, []string{got[0].GUID, got[1].GUID})
}

func TestDebounce_CoalescesBursts(t *testing.T) {
	var n atomic.Int32
	trigger, stop := sysdns.Debounce(100*time.Millisecond, func() { n.Add(1) })
	defer stop()
	for i := 0; i < 5; i++ {
		trigger()
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(250 * time.Millisecond)
	require.Equal(t, int32(1), n.Load())
}

func TestSnapshot_LoopbackIsRecordedAsDHCP(t *testing.T) { // review I1
	api := &fakeAPI{adapters: []sysdns.Adapter{eth("{A}", 1)}, dns: map[string][]string{"{A}|v4": {"127.0.0.1"}, "{A}|v6": {"::1"}}}
	m, _ := newMgr(api)
	ads, _ := m.Select("auto", nil)
	snaps, err := m.Snapshot(ads)
	require.NoError(t, err)
	require.Equal(t, model.FamilyDNS{Mode: model.DNSModeDHCP}, snaps[0].IPv4, "never save Ghostline's own loopback as the original")
	require.Equal(t, model.FamilyDNS{Mode: model.DNSModeDHCP}, snaps[0].IPv6)
}
