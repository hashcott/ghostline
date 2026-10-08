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

// The warnings speak of a system proxy Ghostline wants to set: once the
// user turns it off, or disconnects, they no longer apply.
func TestProxyPhase_SessionWarningsClearWhenNotWanted(t *testing.T) {
	for _, code := range []string{CodeSessionPending, CodeProxyDesktopUnsupported} {
		t.Run(code, func(t *testing.T) {
			err := map[string]error{CodeSessionPending: sysproxy.ErrNoSession, CodeProxyDesktopUnsupported: sysproxy.ErrDesktopUnsupported}[code]
			h := newProxyHarness(t, true)
			h.sp.snapErr = err
			require.NoError(t, h.o.Connect(context.Background()))
			require.Contains(t, warningCodes(h.o.Snapshot()), code)
			h.settings.Proxy.SystemProxy = false
			require.NoError(t, h.o.ReapplyProxy(context.Background()))
			require.NotContains(t, warningCodes(h.o.Snapshot()), code, "system proxy turned off")

			h.settings.Proxy.SystemProxy = true
			require.NoError(t, h.o.ReapplyProxy(context.Background()))
			require.Contains(t, warningCodes(h.o.Snapshot()), code)
			require.NoError(t, h.o.Disconnect(context.Background()))
			require.NotContains(t, warningCodes(h.o.Snapshot()), code, "disconnected")
		})
	}
}
