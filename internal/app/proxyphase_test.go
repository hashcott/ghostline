package app

import (
	"context"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"sync"
	"testing"

	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type fProxy struct {
	r      *rec
	mu     sync.Mutex
	alive  bool
	runs   []ProxyRun
	startE error
}

func (p *fProxy) Start(_ context.Context, run ProxyRun) error {
	if err := p.r.add("proxy.start"); err != nil {
		return err
	}
	if p.startE != nil {
		return p.startE
	}
	p.mu.Lock()
	p.alive, p.runs = true, append(p.runs, run)
	p.mu.Unlock()
	return nil
}
func (p *fProxy) Stop(context.Context) error {
	p.mu.Lock()
	p.alive = false
	p.mu.Unlock()
	return p.r.add("proxy.stop")
}
func (p *fProxy) SelfTest(context.Context) error { return p.r.add("proxy.selftest") }
func (p *fProxy) Alive() bool                    { p.mu.Lock(); defer p.mu.Unlock(); return p.alive }

type fSysProxy struct {
	r        *rec
	states   *fStates
	existing model.WinINETProxy
	ours     bool
	t        *testing.T
}

// Snapshot follows the Backend contract: Ghostline's own leftover address
// comes back as the default ("direct").
func (s *fSysProxy) Snapshot(ours string) (sysproxy.Snapshot, error) {
	w := s.existing
	if w.Server == ours {
		w = model.WinINETProxy{Flags: 1}
	}
	return sysproxy.Snapshot{Backend: "windows", Windows: &w}, s.r.add("sysproxy.snapshot")
}
func (s *fSysProxy) Existing(snap sysproxy.Snapshot) (string, string, bool) {
	w := snap.Windows
	return w.Server, w.AutoconfigURL, w.Server != "" || w.AutoconfigURL != ""
}
func (s *fSysProxy) Watch(func()) (func(), error) { return func() {}, nil }
func (s *fSysProxy) Info() sysproxy.Info          { return sysproxy.Info{Desktop: "Windows", Supported: true} }
func (s *fSysProxy) Apply(addr string) error {
	// Write-ahead invariant: the snapshot is in state.json before any change.
	st, err := s.states.Load()
	require.NoError(s.t, err)
	require.NotNil(s.t, st.SysProxy, "system proxy changed before its snapshot was persisted")
	require.Equal(s.t, addr, st.SysProxy.Ours)
	if err := s.r.add("sysproxy.apply"); err != nil {
		return err
	}
	s.ours = true
	return nil
}
func (s *fSysProxy) IsOurs(string) (bool, error) { return s.ours, nil }
func (s *fSysProxy) RestoreIfOurs(string, sysproxy.Snapshot) (bool, error) {
	if err := s.r.add("sysproxy.restore"); err != nil {
		return false, err
	}
	was := s.ours
	s.ours = false
	return was, nil
}

type fFirewall struct {
	r      *rec
	states *fStates
	t      *testing.T
}

func (f *fFirewall) Add(int) error {
	st, err := f.states.Load()
	require.NoError(f.t, err)
	require.NotNil(f.t, st.Firewall, "firewall rule created before it was persisted")
	return f.r.add("firewall.add")
}
func (f *fFirewall) Delete() error { return f.r.add("firewall.delete") }
func (f *fFirewall) AddNamed(r firewall.Rule) error {
	st, err := f.states.Load()
	require.NoError(f.t, err)
	require.NotNil(f.t, st.Firewall, "firewall rule created before it was persisted")
	require.Contains(f.t, st.Firewall.Rules, r.Name, "firewall rule created before it was persisted")
	return f.r.add("firewall.add:" + r.Name)
}
func (f *fFirewall) DeleteNamed(name string) error { return f.r.add("firewall.delete:" + name) }

type proxyHarness struct {
	*harness
	proxy *fProxy
	sp    *fSysProxy
	fw    *fFirewall
	asked int
}

func newProxyHarness(t *testing.T, confirm bool) *proxyHarness {
	h := newHarness(t)
	ph := &proxyHarness{harness: h,
		proxy: &fProxy{r: h.r},
		sp:    &fSysProxy{r: h.r, states: h.states, t: t},
		fw:    &fFirewall{r: h.r, states: h.states, t: t},
	}
	h.o.d.Proxy, h.o.d.SysProxy, h.o.d.Firewall = ph.proxy, ph.sp, ph.fw
	h.o.d.ConfirmOverride = func(_ context.Context, server, pac string) bool { ph.asked++; return confirm }
	h.settings.Proxy.Enabled = true
	h.settings.Proxy.SystemProxy = true
	h.settings.Proxy.ShareLAN = true
	return ph
}

func requireOrder(t *testing.T, calls []string, names ...string) {
	t.Helper()
	last := -1
	for _, n := range names {
		i := indexOf(calls[last+1:], n)
		require.GreaterOrEqual(t, i, 0, "%s missing after position %d in %v", n, last, calls)
		last += i + 1
	}
}

func TestConnect_ProxyPhaseRunsAfterProtected(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	calls := h.r.list()
	requireOrder(t, calls, "engine.saw", "proxy.start", "proxy.selftest", "sysproxy.snapshot", "firewall.add", "sysproxy.apply")
	sn := h.o.Snapshot()
	require.Equal(t, StatusProtected, sn.Status)
	require.Equal(t, ProxyStatus{Running: true, Addr: "127.0.0.1:8080", SystemProxy: true, ShareLAN: true}, sn.Proxy)
	require.Equal(t, []ProxyRun{{Listen: listenFor(8080, true, true), ShareLAN: true}}, h.proxy.runs)
	st, _ := h.states.Load()
	require.True(t, st.SysProxy.Set)
	require.ElementsMatch(t, []string{firewall.ProxyRule, firewall.RuleBlockPublic}, st.Firewall.Rules)
}

func TestProxyPhase_FailureAtEachStep(t *testing.T) {
	cases := []struct {
		fail, code string
		undo       []string
	}{
		{"proxy.start", CodeProxySelfTest, nil},
		{"proxy.selftest", CodeProxySelfTest, []string{"proxy.stop"}},
		{"sysproxy.snapshot", CodeSysProxyFailed, []string{"proxy.stop"}},
		{"firewall.add", CodeProxyFirewall, []string{"proxy.stop"}},
		{"sysproxy.apply", CodeSysProxyFailed, []string{"firewall.delete", "proxy.stop"}},
	}
	for _, c := range cases {
		t.Run(c.fail, func(t *testing.T) {
			h := newProxyHarness(t, true)
			h.r.fail[c.fail] = true
			require.NoError(t, h.o.Connect(context.Background())) // DNS is still protected
			calls := h.r.list()
			after := calls[indexOf(calls, c.fail)+1:]
			for _, u := range c.undo {
				require.Contains(t, after, u)
			}
			if len(c.undo) == 2 {
				requireOrder(t, after, c.undo...)
			}
			require.NotContains(t, calls, "dns.restore")
			sn := h.o.Snapshot()
			require.Equal(t, StatusDegraded, sn.Status)
			require.Equal(t, []string{"proxy"}, sn.Reasons)
			require.Equal(t, c.code, sn.Proxy.Error.Code)
			require.False(t, sn.Proxy.Running)
			st, _ := h.states.Load()
			require.Nil(t, st.SysProxy)
			require.NotContains(t, firewallRules(st), firewall.ProxyRule, "the proxy rule is undone; only the Public block rule may stay")
			require.Equal(t, store.PhaseDNSSet, st.Phase)
		})
	}
}

func TestProxyPhase_PortBusy(t *testing.T) {
	h := newProxyHarness(t, true)
	h.proxy.startE = errBoom
	h.sys.proxyOwners = []procs.PortOwner{{PID: 4242, Name: "nginx.exe"}}
	require.NoError(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Proxy.Error
	require.Equal(t, CodeProxyPortBusy, e.Code)
	require.Equal(t, map[string]any{"port": 8080, "pid": uint32(4242), "name": "nginx.exe"}, e.Params)
}

func TestProxyPhase_ExistingDeclined(t *testing.T) {
	h := newProxyHarness(t, false)
	h.sp.existing = model.WinINETProxy{Flags: 3, Server: "10.0.0.1:3128"}
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, 1, h.asked)
	require.NotContains(t, h.r.list(), "sysproxy.apply")
	sn := h.o.Snapshot()
	require.Equal(t, StatusProtected, sn.Status)
	require.True(t, sn.Proxy.Running)
	require.False(t, sn.Proxy.SystemProxy)
	st, _ := h.states.Load()
	require.Nil(t, st.SysProxy)
}

func TestDisconnect_Order(t *testing.T) {
	h := newProxyHarness(t, true)
	h.settings.DPI.Enabled = true
	require.NoError(t, h.o.Connect(context.Background()))
	h.dpi.setRunning("goodbyedpi")
	n := len(h.r.list())
	require.NoError(t, h.o.Disconnect(context.Background()))
	requireOrder(t, h.r.list()[n:], "sysproxy.restore", "firewall.delete", "proxy.stop", "dns.restore", "dpi.stop", "engine.stop", "state.clean")
	require.Equal(t, ProxyStatus{}, h.o.Snapshot().Proxy)
}

func TestDisconnect_TakenOverNotRestored(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	h.sp.ours = false // another app replaced our setting
	h.o.OnSysProxyChanged()
	st, _ := h.states.Load()
	require.True(t, st.SysProxy.TakenOver)
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeSysProxyTakenOver)
	n := len(h.r.list())
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.NotContains(t, h.r.list()[n:], "sysproxy.restore")
}

func warningCodes(s Snapshot) []string {
	var out []string
	for _, w := range s.Warnings {
		out = append(out, w.Code)
	}
	return out
}

func TestReasons_UpstreamsAndProxy(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	h.o.addReason(reasonUpstreams)
	h.o.addReason(reasonProxy)
	require.Equal(t, StatusDegraded, h.o.Snapshot().Status)
	h.o.clearReason(reasonUpstreams)
	require.Equal(t, StatusDegraded, h.o.Snapshot().Status)
	require.Equal(t, []string{"proxy"}, h.o.Snapshot().Reasons)
	h.o.clearReason(reasonProxy)
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
	require.Empty(t, h.o.Snapshot().Reasons)
}

func TestProxyPhase_ReapplyRacesDisconnect(t *testing.T) {
	for i := 0; i < 50; i++ {
		h := newProxyHarness(t, true)
		require.NoError(t, h.o.Connect(context.Background()))
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _ = h.o.ReapplyProxy(context.Background()) }()
		go func() { defer wg.Done(); _ = h.o.Disconnect(context.Background()) }()
		wg.Wait()
		calls := h.r.list()
		lastStop := -1
		lastApply := -1
		for j, c := range calls {
			switch c {
			case "proxy.stop":
				lastStop = j
			case "sysproxy.apply":
				lastApply = j
			}
		}
		require.Greater(t, lastStop, lastApply, "system proxy applied after the final proxy stop: %v", calls)
		require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
		require.False(t, h.sp.ours)
	}
}

func TestReapplyProxy_PortChange(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	h.setSettings(func(s *store.Settings) { s.Proxy.Port, s.Proxy.ShareLAN = 9090, false })
	require.NoError(t, h.o.ReapplyProxy(context.Background()))
	require.Equal(t, "127.0.0.1:9090", h.o.Snapshot().Proxy.Addr)
	require.Equal(t, listenFor(9090, false, true), h.proxy.runs[len(h.proxy.runs)-1].Listen)
	st, _ := h.states.Load()
	require.Equal(t, onlyBlockPublic, firewallRules(st), "only the Public block rule may stay until Disconnect")
	require.Equal(t, "127.0.0.1:9090", st.SysProxy.Ours)
}

func TestReapplyProxy_DisabledStopsPhase(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	h.setSettings(func(s *store.Settings) { s.Proxy.Enabled = false })
	require.NoError(t, h.o.ReapplyProxy(context.Background()))
	require.Equal(t, ProxyStatus{}, h.o.Snapshot().Proxy)
	require.False(t, h.proxy.Alive())
	require.False(t, h.sp.ours)
}

func TestHealth_ProxyListenerDiedRestartsOnce(t *testing.T) {
	h := newProxyHarness(t, true)
	require.NoError(t, h.o.Connect(context.Background()))
	h.proxy.mu.Lock()
	h.proxy.alive = false
	h.proxy.mu.Unlock()
	h.o.checkProxyHealth(context.Background())
	require.True(t, h.proxy.Alive())
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)

	h.proxy.mu.Lock()
	h.proxy.alive = false
	h.proxy.mu.Unlock()
	h.r.fail["proxy.start"] = true
	h.o.checkProxyHealth(context.Background())
	require.Equal(t, StatusDegraded, h.o.Snapshot().Status)
	require.Equal(t, []string{"proxy"}, h.o.Snapshot().Reasons)
}

func TestConnect_ProxyDisabledUnchanged(t *testing.T) {
	h := newProxyHarness(t, true)
	h.settings.Proxy.Enabled = false
	require.NoError(t, h.o.Connect(context.Background()))
	for _, c := range h.r.list() {
		require.NotContains(t, []string{"proxy.start", "sysproxy.snapshot", "firewall.add", "sysproxy.apply"}, c)
	}
	require.NoError(t, h.o.Disconnect(context.Background()))
	for _, c := range h.r.list() {
		require.NotContains(t, []string{"proxy.stop", "sysproxy.restore", "firewall.delete"}, c)
	}
}
