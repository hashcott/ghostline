package app

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

var happy = []string{"sys.admin", "sys.listen", "pick", "build", "engine.start", "engine.selftest", "dns.snapshot",
	"state.dns_set", "safety.watchdog", "safety.task.create", "dns.apply", "dns.flush", "engine.expect", "resolve", "engine.saw"}

func TestConnect_HappyPathOrder(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, happy, h.r.list())
	sn := h.o.Snapshot()
	require.Equal(t, StatusProtected, sn.Status)
	require.Equal(t, []string{"Cloudflare"}, sn.Servers)
	require.False(t, sn.Since.IsZero())
	st, _ := h.states.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase)
	require.Equal(t, uint32(1234), st.PID)
	require.Len(t, st.DNS.Windows, 1)
}

func TestConnect_FailureAtEachStepRollsBack(t *testing.T) {
	cases := []struct {
		fail string
		code string
		undo []string
	}{
		{"pick", CodeNoServers, nil},
		{"engine.start", CodeEngineSelfTest, nil},
		{"engine.selftest", CodeEngineSelfTest, []string{"engine.stop", "sys.loopback"}},
		{"dns.snapshot", CodeSetDNSFailed, []string{"engine.stop"}},
		{"safety.watchdog", CodeInternal, []string{"state.clean", "engine.stop"}},
		{"dns.apply", CodeSetDNSFailed, []string{"dns.restore", "dns.flush", "safety.task.delete", "safety.watchdog.stop", "state.clean", "engine.stop"}},
		{"engine.saw", CodeVerifyLeak, []string{"dns.report", "dns.restore", "dns.flush", "safety.task.delete", "safety.watchdog.stop", "state.clean", "engine.stop"}},
	}
	for _, c := range cases {
		t.Run(c.fail, func(t *testing.T) {
			h := newHarness(t)
			h.r.fail[c.fail] = true
			require.Error(t, h.o.Connect(context.Background()))
			calls := h.r.list()
			i := indexOf(calls, c.fail)
			require.GreaterOrEqual(t, i, 0, calls)
			require.Equal(t, c.undo, nilIfEmpty(calls[i+1:]), "calls after failure")
			sn := h.o.Snapshot()
			require.Equal(t, StatusError, sn.Status)
			require.Equal(t, c.code, sn.Error.Code)
			st, _ := h.states.Load()
			require.Equal(t, store.PhaseClean, st.Phase)
		})
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func TestDisconnect_RestoresBeforeEngineStop(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	n := len(h.r.list())
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Equal(t, []string{"dns.restore", "dns.flush", "engine.stop", "state.clean", "safety.watchdog.stop", "safety.task.delete"}, h.r.list()[n:])
	require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
}

func TestDisconnect_StopsRunningDPI(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.dpi.setRunning("goodbyedpi")
	n := len(h.r.list())
	require.NoError(t, h.o.Disconnect(context.Background()))
	require.Equal(t, []string{"dns.restore", "dns.flush", "dpi.stop", "engine.stop"}, h.r.list()[n:n+4])
}

func TestDisconnect_RestoreFailureWarnsAndKeepsStateDirty(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.dns.restoreErr = true
	require.NoError(t, h.o.Disconnect(context.Background()))
	sn := h.o.Snapshot()
	require.Len(t, sn.Warnings, 1)
	require.Equal(t, CodeRestoreFailed, sn.Warnings[0].Code)
	require.Equal(t, "Wi-Fi", sn.Warnings[0].Params["adapter"])
	st, _ := h.states.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase)
	require.NotContains(t, h.r.list(), "engine.stop", "DNS still points at loopback: keep the engine alive")
	require.Equal(t, StatusProtected, sn.Status)
}

func TestConnect_CancelDuringPickRollsBackToDisconnected(t *testing.T) {
	h := newHarness(t)
	h.pick.block = make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- h.o.Connect(context.Background()) }()
	require.Eventually(t, func() bool { return indexOf(h.r.list(), "pick") >= 0 }, time.Second, 5*time.Millisecond)
	h.o.Cancel()
	<-done
	require.Equal(t, StatusDisconnected, h.o.Snapshot().Status)
	require.Nil(t, h.o.Snapshot().Error)
}

func TestConnect_ConcurrentCallsAreSerialized(t *testing.T) { // Review Focus #1
	h := newHarness(t)
	h.pick.block = make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = h.o.Connect(context.Background()) }()
	}
	require.Eventually(t, func() bool { return indexOf(h.r.list(), "pick") >= 0 }, time.Second, 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	close(h.pick.block)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	count := func(name string) int {
		n := 0
		for _, c := range h.r.list() {
			if c == name {
				n++
			}
		}
		return n
	}
	require.Equal(t, 1, count("pick"))
	require.Equal(t, 1, count("dns.snapshot"))
}

func TestConnect_WhileDisconnectingIsNoop(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.o.mu.Lock()
	h.o.snap.Status = StatusDisconnecting
	h.o.mu.Unlock()
	n := len(h.r.list())
	require.NoError(t, h.o.Connect(context.Background()))
	require.Len(t, h.r.list(), n)
}

func TestConnect_NotAdmin(t *testing.T) {
	h := newHarness(t)
	h.sys.admin = false
	require.Error(t, h.o.Connect(context.Background()))
	require.Equal(t, CodeNotAdmin, h.o.Snapshot().Error.Code)
	require.Equal(t, []string{"sys.admin"}, h.r.list())
}

func TestConnect_Port53Busy(t *testing.T) {
	h := newHarness(t)
	h.sys.listenErr = errors.New("bind: access denied")
	h.sys.owners = []procs.PortOwner{{PID: 1234, Name: "svchost.exe", Service: "SharedAccess", Proto: "udp"}}
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, CodePort53Busy, e.Code)
	require.Equal(t, uint32(1234), e.Params["pid"])
	require.Equal(t, "svchost.exe", e.Params["name"])
	require.Equal(t, "SharedAccess", e.Params["service"])
}

// Mobile Hotspot (SharedAccess) holds 0.0.0.0:53, yet 127.0.0.1:53 still
// binds and gets every loopback query: an owner alone is no conflict.
func TestConnect_Port53OwnerButLoopbackFree(t *testing.T) {
	h := newHarness(t)
	h.sys.owners = []procs.PortOwner{{PID: 1892, Name: "svchost.exe", Service: "SharedAccess", Proto: "udp"}}
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, happy, h.r.list())
}

func TestConnect_Port53BusyWithNoKnownOwner(t *testing.T) {
	h := newHarness(t)
	h.sys.listenErr = errors.New("bind: access denied")
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, CodePort53Busy, e.Code)
	require.Equal(t, "?", e.Params["name"])
}

func TestConnect_ProbesTheListenAddresses(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:53"), netip.MustParseAddrPort("[::1]:53")}, h.sys.probed)

	h = newHarness(t)
	h.sys.noV6 = true
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:53")}, h.sys.probed)
}

func TestConnect_DirtyStateRecoversFirst(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.states.s.Update(func(st *store.State) error { st.Phase = store.PhaseDNSSet; return nil }))
	require.NoError(t, h.o.Connect(context.Background()))
	calls := h.r.list()
	require.Less(t, indexOf(calls, "recover"), indexOf(calls, "sys.listen"))
	require.Equal(t, 1, h.recovers)
}

func TestConnect_NoServersParams(t *testing.T) {
	h := newHarness(t)
	h.pick.err = &NoServersError{Checked: 16, Elapsed: 20 * time.Second}
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, CodeNoServers, e.Code)
	require.Equal(t, 16, e.Params["checked"])
	require.Equal(t, int64(20), e.Params["elapsed"])
}

func TestConnect_EmitsSteps(t *testing.T) {
	h := newHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	var steps []int
	for _, s := range h.sink.snapshots() {
		if s.Status == StatusConnecting && (len(steps) == 0 || steps[len(steps)-1] != s.Step) {
			steps = append(steps, s.Step)
		}
	}
	require.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7}, steps)
}

func TestWarnings_AddAndClear(t *testing.T) {
	h := newHarness(t)
	h.o.AddWarning(AppError{Code: CodeSettingsReset})
	h.o.AddWarning(AppError{Code: CodeSettingsReset})
	require.Len(t, h.o.Snapshot().Warnings, 1)
	h.o.ClearWarning(CodeSettingsReset)
	require.Empty(t, h.o.Snapshot().Warnings)
}

func TestConnect_PickProgressInSnapshot(t *testing.T) {
	h := newHarness(t)
	var seen [][2]int
	h.o.d.Picker = progressPicker{h.o.d.Picker, func() { seen = append(seen, [2]int{h.o.Snapshot().PickDone, h.o.Snapshot().PickTotal}) }}
	require.NoError(t, h.o.Connect(context.Background()))
	require.Contains(t, seen, [2]int{900, 900})
	require.Zero(t, h.o.Snapshot().PickTotal, "cleared once connected")
}

// progressPicker reports a 900-server scan, then picks with the real fake.
type progressPicker struct {
	inner Picker
	after func()
}

func (p progressPicker) Pick(ctx context.Context, onProgress func(done, total int)) ([]model.Server, error) {
	for _, d := range []int{1, 450, 900} {
		onProgress(d, 900)
	}
	p.after()
	return p.inner.Pick(ctx, nil)
}

// The engine runs but its self test gets no answer: another program on
// this machine takes the loopback DNS packets (AdGuard, antivirus, VPN).
func TestConnect_SelfTestTimeoutWithDNSIntercepted(t *testing.T) {
	h := newHarness(t)
	h.eng.selfE = errors.New("read udp 127.0.0.1:64590->127.0.0.1:53: i/o timeout")
	h.sys.loopErr = errors.New("no datagram")
	h.sys.procNames = []string{"System", "svchost.exe", "AdguardSvc.exe"}
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, CodeDNSIntercepted, e.Code)
	require.Equal(t, "AdGuard", e.Params["name"])
	require.Equal(t, "adguard", e.Params["hint"])
	// The probe runs once the engine has released port 53.
	require.Equal(t, []string{"engine.stop", "sys.loopback"}, after(h.r.list(), "engine.selftest"))
}

// An interceptor Ghostline does not know by name is still reported.
func TestConnect_SelfTestTimeoutWithUnknownInterceptor(t *testing.T) {
	h := newHarness(t)
	h.eng.selfE = errors.New("i/o timeout")
	h.sys.loopErr = errors.New("no datagram")
	h.sys.procNames = []string{"System", "mystery.exe"}
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, CodeDNSIntercepted, e.Code)
	require.Equal(t, "", e.Params["name"])
	require.Equal(t, "", e.Params["hint"])
}

// Several known interceptors run: all are named, the first gives the hint.
func TestConnect_SelfTestTimeoutWithSeveralInterceptors(t *testing.T) {
	h := newHarness(t)
	h.eng.selfE = errors.New("i/o timeout")
	h.sys.loopErr = errors.New("no datagram")
	h.sys.procNames = []string{"AvastSvc.exe", "Adguard.exe"}
	require.Error(t, h.o.Connect(context.Background()))
	e := h.o.Snapshot().Error
	require.Equal(t, "AdGuard, Avast", e.Params["name"])
	require.Equal(t, "adguard", e.Params["hint"])
}

// Loopback DNS works, so the engine itself failed: the old error stays.
func TestConnect_SelfTestFailsWithLoopbackWorking(t *testing.T) {
	h := newHarness(t)
	h.eng.selfE = errors.New("i/o timeout")
	h.sys.procNames = []string{"AdguardSvc.exe"} // running, DNS protection off
	require.Error(t, h.o.Connect(context.Background()))
	require.Equal(t, CodeEngineSelfTest, h.o.Snapshot().Error.Code)
}

func TestFindDNSInterceptors(t *testing.T) {
	names := func(ds []dnsInterceptor) []string {
		var out []string
		for _, d := range ds {
			out = append(out, d.name)
		}
		return out
	}
	require.Equal(t, []string{"AdGuard"}, names(findDNSInterceptors([]string{"adguard.exe", "AdguardSvc.exe"})))
	require.Equal(t, []string{"AdGuard"}, names(findDNSInterceptors([]string{`C:\Program Files\AdGuard\AdguardSvc.exe`})))
	require.Equal(t, []string{"Avast", "Portmaster"}, names(findDNSInterceptors([]string{"portmaster-core_v1-6-10.exe", "AvastSvc.exe"})))
	require.Equal(t, []string{"YogaDNS"}, names(findDNSInterceptors([]string{"YogaDNS.exe"})))
	require.Empty(t, findDNSInterceptors([]string{"chrome.exe", "AdGuardHome.exe", "portmaster.exe"}))
	require.Empty(t, findDNSInterceptors(nil))
}
