package app

import (
	"context"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

func TestService_DPIInfo(t *testing.T) {
	sh := newSvc(t)
	require.Equal(t, DPIInfo{}, sh.svc.DPIInfo(), "no source: empty, not a crash")
	sh.svc.x.DPIInfo = func() DPIInfo {
		return DPIInfo{Engines: []DPIEngineInfo{{ID: "zapret2", Exe: "nfqws2"}}, Mechanism: "nftables"}
	}
	require.Equal(t, "nfqws2", sh.svc.DPIInfo().Engines[0].Exe)
}

// Settings imported from Windows may name GoodbyeDPI, which Linux lacks.
func TestStartDPI_UnavailableEngineUsesZapret2(t *testing.T) {
	h := newHarness(t)
	h.setSettings(func(s *store.Settings) { s.DPI.Engine = store.EngineGoodbyeDPI })
	h.dpi.missing = map[string]bool{store.EngineGoodbyeDPI: true}
	require.NoError(t, h.o.Connect(context.Background()))
	require.NoError(t, h.o.SetDPIEnabled(context.Background(), true))
	require.Equal(t, store.EngineZapret2, h.dpi.lastStart().engine)
}

func TestService_SysProxyInfo(t *testing.T) {
	sh := newSvc(t)
	require.Equal(t, sysproxy.Info{}, sh.svc.SysProxyInfo())
	sh.svc.x.SysProxyInfo = func() sysproxy.Info { return sysproxy.Info{Desktop: "KDE", Supported: true} }
	require.Equal(t, "KDE", sh.svc.SysProxyInfo().Desktop)
}
