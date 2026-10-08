package app

import (
	"context"
	"testing"

	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func countCalls(calls []string, name string) int {
	n := 0
	for _, c := range calls {
		if c == name {
			n++
		}
	}
	return n
}

const addBlock = "firewall.add:" + firewall.RuleBlockPublic
const delBlock = "firewall.delete:" + firewall.RuleBlockPublic

// Sharing on the LAN adds the Public block rule (recorded in state.json
// first, checked by the fake) and Disconnect removes it.
func TestBlockPublic_WithProxyShare(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, 1, countCalls(h.r.list(), addBlock))
	st, _ := h.states.Load()
	require.Contains(t, st.Firewall.Rules, firewall.RuleBlockPublic)

	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Contains(t, h.r.list(), delBlock)
	st, _ = h.states.Load()
	require.Nil(t, st.Firewall)
}

func TestBlockPublic_WithDNSShareOnly(t *testing.T) {
	h := newDNSHarness(t)
	h.settings.Proxy.ShareLAN = false
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, 1, countCalls(h.r.list(), addBlock))
}

func TestBlockPublic_OnceForBothShares(t *testing.T) {
	h := newDNSHarness(t) // proxy and DNS server both shared
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, 1, countCalls(h.r.list(), addBlock))
	// Restarting the proxy phase keeps the rule (the DNS server still shares).
	require.NoError(t, h.o.ReapplyProxy(context.Background()))
	require.Zero(t, countCalls(h.r.list(), delBlock))
}

func TestBlockPublic_NotWithoutSharing(t *testing.T) {
	h := newProxyHarness(t, true)
	h.settings.Proxy.ShareLAN = false
	require.NoError(t, h.o.Connect(context.Background()))
	require.NotContains(t, h.r.list(), addBlock)
}

// If the block rule cannot be created, LAN sharing is not opened.
func TestBlockPublic_FailureStopsSharing(t *testing.T) {
	h := newProxyHarness(t, true)
	h.r.fail[addBlock] = true
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, StatusDegraded, sn.Status)
	require.Equal(t, CodeProxyFirewall, sn.Proxy.Error.Code)
}

var onlyBlockPublic = []string{firewall.RuleBlockPublic}

func firewallRules(st store.State) []string {
	if st.Firewall == nil {
		return nil
	}
	return st.Firewall.Rules
}
