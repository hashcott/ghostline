package app

import (
	"context"
	"testing"

	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

// Booting with autostart, before anyone logs in: Connect still protects the
// machine; the system proxy waits for a desktop session.
func TestProxyPhase_NoSessionWarnsAndConnects(t *testing.T) {
	h := newProxyHarness(t, true)
	h.sp.snapErr = sysproxy.ErrNoSession
	require.NoError(t, h.o.Connect(context.Background()))
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeSessionPending)
	require.NotContains(t, warningCodes(h.o.Snapshot()), CodeSysProxyFailed)
	require.NotContains(t, h.r.list(), "sysproxy.apply")
	require.True(t, h.o.Snapshot().Proxy.Running)
}

func TestOnSessionNew_ReappliesWhenPending(t *testing.T) {
	h := newProxyHarness(t, true)
	h.sp.snapErr = sysproxy.ErrNoSession
	require.NoError(t, h.o.Connect(context.Background()))
	h.sp.snapErr = nil // the user logged in
	h.o.OnSessionNew(context.Background())
	require.Contains(t, h.r.list(), "sysproxy.apply")
	require.NotContains(t, warningCodes(h.o.Snapshot()), CodeSessionPending)

	n := len(h.r.list())
	h.o.OnSessionNew(context.Background()) // nothing pending: nothing to do
	require.NotContains(t, h.r.list()[n:], "sysproxy.apply")
}

func TestProxyPhase_DesktopUnsupportedWarns(t *testing.T) {
	h := newProxyHarness(t, true)
	h.sp.snapErr = sysproxy.ErrDesktopUnsupported
	require.NoError(t, h.o.Connect(context.Background()))
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeProxyDesktopUnsupported)
	require.NotContains(t, h.r.list(), "sysproxy.apply")
}
