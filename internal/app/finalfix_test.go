package app

import (
	"context"
	"github.com/hashcott/ghostline/internal/model"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

// Final review C2: an unrelated settings save from the UI (which carries a
// stale or masked copy of the upstreams) must not replace stored upstreams.
func TestSaveSettings_KeepsStoredUpstreams(t *testing.T) {
	rh := newRulesSvc(t)
	rh.svc.x.Protect = fakeProtect
	require.NoError(t, rh.svc.SaveUpstreamProxy(store.UpstreamProxy{ID: "tor", Type: "socks5", Addr: "127.0.0.1:9050"}, "pw"))
	ui := rh.svc.GetSettings()
	ui.Proxy.Upstreams = []store.UpstreamProxy{{ID: "tor", Type: "socks5", Addr: "127.0.0.1:9050", PassEnc: "•"}, {ID: "ghost", Type: "http", Addr: "1.2.3.4:8080"}}
	ui.Proxy.Fragment.Chunks = 7
	require.NoError(t, rh.svc.SaveSettings(ui))
	got := rh.svc.GetSettings()
	require.Equal(t, 7, got.Proxy.Fragment.Chunks)
	require.Equal(t, []store.UpstreamProxy{{ID: "tor", Type: "socks5", Addr: "127.0.0.1:9050", PassEnc: "enc:PW"}}, got.Proxy.Upstreams)
}

// Final review I5: after SYSPROXY_RESTORE_FAILED the snapshot is kept and
// the Restore system proxy action can still put it back.
func TestRestoreSystemProxy_AfterFailedRestore(t *testing.T) {
	ph := newProxyHarness(t, true)
	sh := newSvc(t)
	sh.svc.o = ph.o
	require.NoError(t, ph.o.Connect(context.Background()))
	ph.r.fail["sysproxy.restore"] = true
	require.NoError(t, ph.o.Disconnect(context.Background()))
	require.Contains(t, warningCodes(ph.o.Snapshot()), CodeSysProxyRestore)

	delete(ph.r.fail, "sysproxy.restore")
	require.NoError(t, sh.svc.RestoreSystemProxy())
	require.NotContains(t, warningCodes(ph.o.Snapshot()), CodeSysProxyRestore)
	require.False(t, ph.sp.ours)
	require.Error(t, sh.svc.RestoreSystemProxy()) // nothing left to restore
}

// Final review I7: a leftover Ghostline value (crash before Set was
// recorded) is not "another app's proxy": no prompt, and the snapshot that
// gets restored later is a direct connection, not the dead port.
func TestProxyPhase_StaleOwnValueNotPrompted(t *testing.T) {
	ph := newProxyHarness(t, true)
	ph.sp.existing = model.WinINETProxy{Flags: 3, Server: "127.0.0.1:8080"}
	require.NoError(t, ph.o.Connect(context.Background()))
	require.Zero(t, ph.asked)
	st, _ := ph.states.Load()
	require.Equal(t, model.ProxySnapshot{Backend: "windows", Windows: &model.WinINETProxy{Flags: 1}}, *st.SysProxy.Snapshot)
}

// Final review I9: Disconnect must not wait up to 60 s for an unanswered
// SYSPROXY_EXISTING prompt.
func TestDisconnect_CancelsPendingOverridePrompt(t *testing.T) {
	ph := newProxyHarness(t, true)
	ph.sp.existing = model.WinINETProxy{Flags: 3, Server: "10.0.0.1:3128"}
	ph.o.d.ConfirmOverride = func(ctx context.Context, server, pac string) bool {
		select {
		case <-ctx.Done():
			return false
		case <-time.After(30 * time.Second):
			return true
		}
	}
	connected := make(chan struct{})
	go func() { _ = ph.o.Connect(context.Background()); close(connected) }()
	require.Eventually(t, func() bool { return indexOf(ph.r.list(), "sysproxy.snapshot") >= 0 }, 2*time.Second, 5*time.Millisecond)
	start := time.Now()
	require.NoError(t, ph.o.Disconnect(context.Background()))
	require.Less(t, time.Since(start), 3*time.Second)
	<-connected
	require.Equal(t, StatusDisconnected, ph.o.Snapshot().Status)
}
