package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func failingStats() engine.Stats {
	return engine.Stats{PerUpstream: map[string]engine.UpstreamStat{"nop": {Errors: 3, LastErrAt: time.Unix(200, 0), LastOKAt: time.Unix(100, 0)}}}
}

func TestHealth_DegradesAfter15sAndSwaps(t *testing.T) {
	h := newHarness(t)
	ck := &clock{t: time.Unix(1000, 0)}
	h.o.d.Now = ck.now
	ticks := make(chan time.Time)
	h.o.d.Ticker = func(time.Duration) (<-chan time.Time, func()) { return ticks, func() {} }
	require.NoError(t, h.o.Connect(context.Background()))
	h.eng.mu.Lock()
	h.eng.stats = failingStats()
	h.eng.mu.Unlock()

	ticks <- ck.t // first failure observed at t=1000
	ck.t = ck.t.Add(10 * time.Second)
	ticks <- ck.t
	require.Never(t, func() bool { h.eng.mu.Lock(); defer h.eng.mu.Unlock(); return h.eng.swaps > 0 }, 50*time.Millisecond, 5*time.Millisecond)

	ck.t = ck.t.Add(10 * time.Second) // 20s of continuous failure
	ticks <- ck.t
	require.Eventually(t, func() bool { h.eng.mu.Lock(); defer h.eng.mu.Unlock(); return h.eng.swaps == 1 }, time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return h.o.Snapshot().Status == StatusProtected }, time.Second, 5*time.Millisecond)
	require.NoError(t, h.o.Disconnect(context.Background()))
}

func TestHealth_RecoversWithoutSwapWhenUpstreamsComeBack(t *testing.T) {
	h := newHarness(t)
	ticks := make(chan time.Time)
	h.o.d.Ticker = func(time.Duration) (<-chan time.Time, func()) { return ticks, func() {} }
	require.NoError(t, h.o.Connect(context.Background()))
	h.eng.mu.Lock()
	h.eng.stats = failingStats()
	h.eng.mu.Unlock()
	ticks <- time.Now()
	h.eng.mu.Lock()
	h.eng.stats = engine.Stats{}
	h.eng.mu.Unlock()
	ticks <- time.Now().Add(time.Minute)
	require.Never(t, func() bool { h.eng.mu.Lock(); defer h.eng.mu.Unlock(); return h.eng.swaps > 0 }, 50*time.Millisecond, 5*time.Millisecond)
	require.NoError(t, h.o.Disconnect(context.Background()))
}

func TestNetworkChange_NewAdapterSnapshottedBeforeApply(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.dns.adapters = append(h.dns.adapters, sysdns.Adapter{GUID: "{B}", Alias: "Ethernet", IfType: 6, Up: true, HasGateway: true})
	n := len(h.r.list())
	h.o.OnNetworkChange(context.Background())
	after := h.r.list()[n:]
	require.Less(t, indexOf(after, "state.append"), indexOf(after, "dns.apply:{B}"), after)
	st, _ := h.states.Load()
	require.Len(t, st.DNS.Windows, 2)
	// Disconnect restores both adapters.
	require.NoError(t, h.o.Disconnect(context.Background()))
}

func TestNetworkChange_IgnoredWhenDisconnected(t *testing.T) {
	h := newHarness(t)
	h.o.OnNetworkChange(context.Background())
	require.Empty(t, h.r.list())
}

func TestResume_SelfTestFailureTriggersSwap(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.eng.mu.Lock()
	h.eng.selfE = errors.New("dead after sleep")
	h.eng.mu.Unlock()
	h.o.OnResume(context.Background())
	require.Contains(t, h.r.list(), "engine.swap")
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
}

func TestPostConnectProbe_SetsBlockedSitesOnlyForTLS(t *testing.T) {
	h := newHarness(t)
	h.prober.stage = func(site string, call int) probe.Stage {
		switch site {
		case "youtube.com":
			return probe.StageTLS
		case "discord.com":
			if call == 1 {
				return probe.StageTLS
			}
		case "x.com":
			return probe.StageTCP
		}
		return probe.StageOK
	}
	require.NoError(t, h.o.Connect(context.Background()))
	require.Eventually(t, func() bool { return len(h.o.Snapshot().BlockedSites) > 0 }, time.Second, 5*time.Millisecond)
	require.Equal(t, []string{"youtube.com"}, h.o.Snapshot().BlockedSites)
}

func TestAutotune_StopsAtFirstWorkingPreset(t *testing.T) {
	h := newHarness(t)
	h.prober.stage = func(site string, call int) probe.Stage {
		if h.dpi.Running() && len(h.dpi.startedArgs()) > 8 { // medium has more args than light
			return probe.StageOK
		}
		return probe.StageTLS
	}
	require.NoError(t, h.o.Connect(context.Background()))
	require.Eventually(t, func() bool { return len(h.o.Snapshot().BlockedSites) > 0 }, time.Second, 5*time.Millisecond)
	var progress []string
	require.NoError(t, h.o.Autotune(context.Background(), func(_, p string, i, n int) {
		progress = append(progress, p)
		require.Equal(t, 4, n)
	}))
	require.Equal(t, []string{"light", "medium"}, progress)
	require.Equal(t, "medium", h.getSettings().DPI.Preset)
	require.True(t, h.getSettings().DPI.Enabled)
	require.True(t, h.dpi.Running())
	sn := h.o.Snapshot()
	require.Empty(t, sn.BlockedSites)
	require.True(t, sn.DPI.Running)
	require.Equal(t, "medium", sn.DPI.Preset)
}

func TestAutotune_NoneWork(t *testing.T) {
	h := newHarness(t)
	h.prober.stage = func(string, int) probe.Stage { return probe.StageTLS }
	require.NoError(t, h.o.Connect(context.Background()))
	err := h.o.Autotune(context.Background(), nil)
	var ae *AppError
	require.True(t, errors.As(err, &ae))
	require.Equal(t, CodeAutotuneNoPreset, ae.Code)
	var lastDPI string
	for _, c := range h.r.list() {
		if strings.HasPrefix(c, "dpi.") {
			lastDPI = c
		}
	}
	require.Equal(t, "dpi.stop", lastDPI)
	require.False(t, h.dpi.Running())
}

func TestAutotune_BlockedByAVStopsImmediately(t *testing.T) {
	h := newHarness(t)
	h.dpi.startE = dpi.ErrBlockedByAV
	require.NoError(t, h.o.Connect(context.Background()))
	err := h.o.Autotune(context.Background(), nil)
	var ae *AppError
	require.True(t, errors.As(err, &ae))
	require.Equal(t, CodeDPIBlockedByAV, ae.Code)
	n := 0
	for _, c := range h.r.list() {
		if c == "dpi.start" {
			n++
		}
	}
	require.Equal(t, 1, n)
}

func TestSetDPIEnabled_MapsHashMismatch(t *testing.T) {
	h := newHarness(t)
	h.dpi.startE = dpi.ErrHashMismatch
	require.NoError(t, h.o.Connect(context.Background()))
	err := h.o.SetDPIEnabled(context.Background(), true)
	var ae *AppError
	require.True(t, errors.As(err, &ae))
	require.Equal(t, CodeDPIHashMismatch, ae.Code)
	require.False(t, h.getSettings().DPI.Enabled)
}

func TestSetDPIEnabled_OnAndOff(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.True(t, h.getSettings().DPI.Enabled)
	require.True(t, h.o.Snapshot().DPI.Running)
	require.Equal(t, []string{"-p", "-r", "-s", "-m", "-e", "40", "-w", "--native-frag"}, h.dpi.startedArgs())
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), false))
	require.False(t, h.getSettings().DPI.Enabled)
	require.False(t, h.o.Snapshot().DPI.Running)
}

func TestSetDPIEnabled_WhileDisconnectedOnlySaves(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.True(t, h.getSettings().DPI.Enabled)
	require.NotContains(t, h.r.list(), "dpi.start")
	require.NoError(t, h.o.Connect(context.Background()))
	require.Contains(t, h.r.list(), "dpi.start", "enabled DPI starts on connect")
}

var _ = store.DefaultSettings

// Linux: the system changed DNS while connected and the backend set it
// again. The log says so, naming what was set.
func TestOnNetworkChange_ReappliedLogsDNSReapplied(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.dns.reapply = []string{"NetworkManager"}
	h.o.OnNetworkChange(context.Background())
	var found bool
	for _, e := range h.sink.events() {
		if e.Code == CodeDNSReapplied && e.Params["adapter"] == "NetworkManager" {
			found = true
		}
	}
	require.True(t, found)
	require.NoError(t, h.o.Disconnect(context.Background()))
}

// The new snapshot is in state.json before the backend applies it, so a
// crash in between never leaves an unrecorded change.
func TestOnNetworkChange_PersistsBeforeApplying(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.dns.adapters = append(h.dns.adapters, sysdns.Adapter{GUID: "{C}", Alias: "USB", IfType: 6, Up: true, HasGateway: true})
	n := len(h.r.list())
	h.o.OnNetworkChange(context.Background())
	after := h.r.list()[n:]
	require.Less(t, indexOf(after, "dns.reconcile"), indexOf(after, "state.append"), after)
	require.Less(t, indexOf(after, "state.append"), indexOf(after, "dns.apply:{C}"), after)
	require.NoError(t, h.o.Disconnect(context.Background()))
}
