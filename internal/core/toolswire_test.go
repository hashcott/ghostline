package core

import (
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func snapWith(servers ...string) []model.AdapterSnapshot {
	return []model.AdapterSnapshot{{GUID: "g", IPv4: model.FamilyDNS{Mode: model.DNSModeStatic, Servers: servers}}}
}

func TestISPResolvers_FromSnapshotWhenConnected(t *testing.T) {
	st := store.State{Phase: "dns_set", DNS: model.DNSSnapshot{Backend: "windows", Windows: snapWith("203.162.4.191")}}
	live := []netid.LiveAdapter{{DNS: []string{"127.0.0.1"}, Gateway: "192.168.1.1"}}
	require.Equal(t, []string{"203.162.4.191"}, ispResolvers(st, live))
}

func TestISPResolvers_FromLiveAdaptersWhenClean(t *testing.T) {
	st := store.State{Phase: "clean"}
	live := []netid.LiveAdapter{{DNS: []string{"203.113.131.1", "203.113.131.2"}, Gateway: "192.168.1.1"}}
	require.Equal(t, []string{"203.113.131.1", "203.113.131.2"}, ispResolvers(st, live))
}

func TestISPResolvers_GatewayWhenNothingElse(t *testing.T) {
	// Connected with a DHCP adapter: the snapshot has no static servers and
	// the live DNS is Ghostline itself. The router usually forwards to the ISP.
	st := store.State{Phase: "dns_set", DNS: model.DNSSnapshot{Backend: "windows", Windows: []model.AdapterSnapshot{{GUID: "g", IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}}}}}
	live := []netid.LiveAdapter{{DNS: []string{"127.0.0.1", "::1"}, Gateway: "192.168.1.1"}}
	require.Equal(t, []string{"192.168.1.1"}, ispResolvers(st, live))
}

func TestISPResolvers_DropsLoopbackLinkLocalAndDupes(t *testing.T) {
	st := store.State{Phase: "clean"}
	live := []netid.LiveAdapter{
		{DNS: []string{"127.0.0.1", "fec0:0:0:ffff::1", "fe80::1", "8.8.8.8"}},
		{DNS: []string{"8.8.8.8", "2001:4860:4860::8888"}},
	}
	require.Equal(t, []string{"8.8.8.8", "2001:4860:4860::8888"}, ispResolvers(st, live))
}

func TestISPResolvers_IPv4First(t *testing.T) {
	live := []netid.LiveAdapter{{DNS: []string{"2001:ee0:23::23", "2001:ee0:26::26", "123.23.23.23", "123.26.26.26"}}}
	require.Equal(t, []string{"123.23.23.23", "123.26.26.26", "2001:ee0:23::23", "2001:ee0:26::26"}, ispResolvers(store.State{Phase: "clean"}, live))
}

// Linux keeps the original servers in the snapshot's Linux branch.
func TestISPResolvers_LinuxSnapshot(t *testing.T) {
	st := store.State{Phase: store.PhaseDNSSet, DNS: model.DNSSnapshot{Backend: "resolved", Linux: &model.LinuxDNS{Servers: []string{"1.0.0.1"}}}}
	require.Equal(t, []string{"1.0.0.1"}, ispResolvers(st, nil))
}
