package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hashcott/ghostline/internal/sysproxy"
	"net/netip"
	"slices"

	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/store"
)

const (
	reasonUpstreams = "upstreams"
	reasonProxy     = "proxy"
)

// proxyState is what the proxy phase changed; guarded by opMu.
type proxyState struct {
	running   bool
	addr      string // the system proxy value Ghostline sets
	sysSet    bool
	takenOver bool
	snap      *sysproxy.Snapshot
	fwSet     bool
}

// pendingRestore keeps a system proxy snapshot whose restore failed, for
// the "Restore system proxy" action (SYSPROXY_RESTORE_FAILED).
type pendingRestore struct {
	addr string
	snap sysproxy.Snapshot
}

// addReason marks the connection degraded for reason r.
func (o *Orchestrator) addReason(r string) {
	o.update(func(s *Snapshot) {
		if s.Status != StatusProtected && s.Status != StatusDegraded {
			return
		}
		if !slices.Contains(s.Reasons, r) {
			s.Reasons = append(s.Reasons, r)
		}
		s.Status = StatusDegraded
	})
}

// clearReason removes r; with no reason left the connection is protected.
func (o *Orchestrator) clearReason(r string) {
	o.update(func(s *Snapshot) {
		s.Reasons = slices.DeleteFunc(s.Reasons, func(x string) bool { return x == r })
		if len(s.Reasons) == 0 && s.Status == StatusDegraded {
			s.Status = StatusProtected
		}
	})
}

// listenFor is where the proxy listens: loopback, or every address when
// sharing on the LAN.
func listenFor(port int, shareLAN, v6 bool) []netip.AddrPort {
	p := uint16(port)
	if shareLAN {
		out := []netip.AddrPort{netip.AddrPortFrom(netip.IPv4Unspecified(), p)}
		if v6 {
			out = append(out, netip.AddrPortFrom(netip.IPv6Unspecified(), p))
		}
		return out
	}
	out := []netip.AddrPort{netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), p)}
	if v6 {
		out = append(out, netip.AddrPortFrom(netip.IPv6Loopback(), p))
	}
	return out
}

func (o *Orchestrator) setSysProxyState(fn func(st *store.State)) error {
	return o.d.States.Update(func(st *store.State) error {
		if st.Phase != store.PhaseDNSSet {
			return errNoChange
		}
		fn(st)
		return nil
	})
}

func ignoreNoChange(err error) error {
	if errors.Is(err, errNoChange) {
		return nil
	}
	return err
}

// startProxyPhase runs P1–P4 (spec 2A 6.1) after DNS is protected. A
// failure undoes the proxy steps and marks the connection degraded; DNS is
// never touched. Callers hold opMu.
func (o *Orchestrator) startProxyPhase(ctx context.Context) error {
	s := o.d.Settings()
	if o.d.Proxy == nil || !s.Proxy.Enabled {
		o.update(func(sn *Snapshot) { sn.Proxy = ProxyStatus{} })
		o.clearReason(reasonProxy)
		return nil
	}
	port := s.Proxy.Port
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	o.mu.Lock()
	v6 := o.v6
	o.mu.Unlock()
	wantSys := s.Proxy.SystemProxy && o.d.SysProxy != nil
	wantFW := s.Proxy.ShareLAN && o.d.Firewall != nil
	var snap sysproxy.Snapshot
	skipSys := false
	// Disconnect cancels the background context before taking opMu, so an
	// unanswered SYSPROXY_EXISTING prompt never blocks it.
	askCtx, cancelAsk := o.background(ctx)
	defer cancelAsk()

	steps := []step{
		{name: "proxy", do: func(ctx context.Context) error {
			if err := o.d.Proxy.Start(ctx, ProxyRun{Listen: listenFor(port, s.Proxy.ShareLAN, v6), ShareLAN: s.Proxy.ShareLAN}); err != nil {
				if owners, _ := o.d.System.PortOwners(uint16(port)); len(owners) > 0 {
					return appErr(CodeProxyPortBusy, err, "port", port, "pid", owners[0].PID, "name", owners[0].Name)
				}
				return appErr(CodeProxySelfTest, err)
			}
			if err := o.d.Proxy.SelfTest(ctx); err != nil {
				warnIgnored("proxy stop", o.d.Proxy.Stop(context.WithoutCancel(ctx)))
				return appErr(CodeProxySelfTest, err)
			}
			o.px.running = true
			return nil
		}, undo: func(ctx context.Context) error {
			o.px.running = false
			return o.d.Proxy.Stop(ctx)
		}},
		{name: "sysproxy.snapshot", do: func(context.Context) error {
			if !wantSys {
				return nil
			}
			var err error
			// Our own leftover value (crash before Set was recorded) comes
			// back as the system default: what was there before is unknown.
			if snap, err = o.d.SysProxy.Snapshot(addr); err != nil {
				switch {
				case errors.Is(err, sysproxy.ErrNoSession):
					// Nobody is logged in yet: protect now, set the
					// system proxy when a desktop session appears.
					skipSys = true
					o.AddWarning(AppError{Code: CodeSessionPending})
					return nil
				case errors.Is(err, sysproxy.ErrDesktopUnsupported):
					skipSys = true
					o.AddWarning(AppError{Code: CodeProxyDesktopUnsupported})
					return nil
				}
				return appErr(CodeSysProxyFailed, err)
			}
			o.ClearWarning(CodeSessionPending)
			o.ClearWarning(CodeProxyDesktopUnsupported)
			if server, pac, has := o.d.SysProxy.Existing(snap); has {
				if o.d.ConfirmOverride == nil || !o.d.ConfirmOverride(askCtx, server, pac) {
					skipSys = true
					return nil
				}
			}
			if err := ignoreNoChange(o.setSysProxyState(func(st *store.State) {
				st.SysProxy = &store.SysProxyState{Ours: addr, Snapshot: &snap}
			})); err != nil {
				return appErr(CodeSysProxyFailed, err)
			}
			o.px.snap, o.px.addr = &snap, addr
			return nil
		}, undo: func(context.Context) error {
			o.px.snap = nil
			return ignoreNoChange(o.setSysProxyState(func(st *store.State) { st.SysProxy = nil }))
		}},
		{name: "firewall", do: func(context.Context) error {
			if !wantFW {
				return nil
			}
			// Block Public before anything listens on the LAN.
			if err := o.ensureBlockPublic(); err != nil {
				return appErr(CodeProxyFirewall, err, "detail", err.Error())
			}
			if err := ignoreNoChange(o.setSysProxyState(func(st *store.State) {
				st.AddFirewallRule(firewall.ProxyRule)
			})); err != nil {
				return appErr(CodeProxyFirewall, err, "detail", err.Error())
			}
			if err := o.d.Firewall.Add(port); err != nil {
				warnIgnored("state update", ignoreNoChange(o.setSysProxyState(func(st *store.State) { st.RemoveFirewallRule(firewall.ProxyRule) })))
				return appErr(CodeProxyFirewall, err, "detail", err.Error())
			}
			o.px.fwSet = true
			return nil
		}, undo: func(context.Context) error {
			if !o.px.fwSet {
				return nil
			}
			o.px.fwSet = false
			err := o.d.Firewall.Delete()
			return errors.Join(err, ignoreNoChange(o.setSysProxyState(func(st *store.State) { st.RemoveFirewallRule(firewall.ProxyRule) })))
		}},
		{name: "sysproxy.apply", do: func(context.Context) error {
			if !wantSys || skipSys {
				return nil
			}
			if err := o.d.SysProxy.Apply(addr); err != nil {
				// Apply may have written before failing to read back.
				_, rerr := o.d.SysProxy.RestoreIfOurs(addr, snap)
				warnIgnored("sysproxy restore", rerr)
				return appErr(CodeSysProxyFailed, err)
			}
			o.px.sysSet, o.px.takenOver = true, false
			if err := ignoreNoChange(o.setSysProxyState(func(st *store.State) {
				if st.SysProxy != nil {
					st.SysProxy.Set = true
				}
			})); err != nil {
				_, rerr := o.d.SysProxy.RestoreIfOurs(addr, snap)
				warnIgnored("sysproxy restore", rerr)
				o.px.sysSet = false
				return appErr(CodeSysProxyFailed, err)
			}
			return nil
		}},
	}
	if err := runSteps(ctx, steps, func(int) {}); err != nil {
		var ae *AppError
		if !errors.As(err, &ae) {
			ae = appErr(CodeProxySelfTest, err)
		}
		o.px = proxyState{}
		o.update(func(sn *Snapshot) { sn.Proxy = ProxyStatus{Error: &AppError{Code: ae.Code, Params: ae.Params}} })
		o.addReason(reasonProxy)
		o.logAppErr("proxy", ae)
		return err
	}
	o.update(func(sn *Snapshot) {
		sn.Proxy = ProxyStatus{Running: true, Addr: addr, SystemProxy: o.px.sysSet, ShareLAN: s.Proxy.ShareLAN}
	})
	o.clearReason(reasonProxy)
	o.log("proxy", "PROXY_STARTED", "addr", addr)
	return nil
}

// stopProxyPhase undoes the proxy phase: system proxy (unless another app
// took it over), firewall rule, then the proxy itself. Callers hold opMu.
func (o *Orchestrator) stopProxyPhase(ctx context.Context) {
	px := o.px
	if px.sysSet && !px.takenOver && px.snap != nil {
		if _, err := o.d.SysProxy.RestoreIfOurs(px.addr, *px.snap); err != nil {
			o.mu.Lock()
			o.pending = &pendingRestore{addr: px.addr, snap: *px.snap}
			o.mu.Unlock()
			o.AddWarning(AppError{Code: CodeSysProxyRestore})
			o.logErr("proxy", CodeSysProxyRestore, err)
		}
	}
	if px.fwSet {
		if err := o.d.Firewall.Delete(); err != nil {
			o.logErr("proxy", CodeProxyFirewall, err, "detail", err.Error())
		}
	}
	if px.running {
		warnIgnored("proxy stop", o.d.Proxy.Stop(ctx))
	}
	if px.running || px.sysSet || px.fwSet || px.snap != nil {
		warnIgnored("state update", ignoreNoChange(o.setSysProxyState(func(st *store.State) {
			st.SysProxy = nil
			st.RemoveFirewallRule(firewall.ProxyRule)
		})))
	}
	o.px = proxyState{}
	o.update(func(sn *Snapshot) { sn.Proxy = ProxyStatus{} })
	o.clearReason(reasonProxy)
	// They spoke of a system proxy this phase wanted; a restart that
	// still wants it adds them again.
	o.ClearWarning(CodeSessionPending)
	o.ClearWarning(CodeProxyDesktopUnsupported)
}

// OnSessionNew runs when a user logs in: a system proxy that waited for a
// desktop session is set now.
func (o *Orchestrator) OnSessionNew(ctx context.Context) {
	pending := false
	for _, w := range o.Snapshot().Warnings {
		pending = pending || w.Code == CodeSessionPending
	}
	if pending {
		_ = o.ReapplyProxy(ctx)
	}
}

// ReapplyProxy restarts the proxy phase after its settings changed. It does
// nothing while not connected.
func (o *Orchestrator) ReapplyProxy(ctx context.Context) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return nil
	}
	// Fake SNI is bound to the running proxy: stop it first, rerun after.
	o.stopSNIPhase(ctx)
	o.stopProxyPhase(ctx)
	err := o.startProxyPhase(ctx)
	_ = o.startSNIPhase(ctx)
	return err
}

// checkProxyHealth restarts a proxy whose listener died, once per check; a
// failed restart leaves the connection degraded.
func (o *Orchestrator) checkProxyHealth(ctx context.Context) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !o.px.running || o.d.Proxy.Alive() {
		return
	}
	o.log("proxy", "PROXY_RESTART")
	o.stopSNIPhase(ctx)
	o.stopProxyPhase(ctx)
	_ = o.startProxyPhase(ctx)
	_ = o.startSNIPhase(ctx)
}

// OnSysProxyChanged reacts to a change of the Windows proxy settings: if
// Ghostline's value was replaced, it is never fought over or restored.
func (o *Orchestrator) OnSysProxyChanged() {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !o.px.sysSet || o.px.takenOver {
		return
	}
	ours, err := o.d.SysProxy.IsOurs(o.px.addr)
	if err != nil {
		slog.Warn("proxy: reading the system proxy failed", "err", err)
		return
	}
	if ours {
		return
	}
	o.px.takenOver = true
	warnIgnored("state update", ignoreNoChange(o.setSysProxyState(func(st *store.State) {
		if st.SysProxy != nil {
			st.SysProxy.TakenOver = true
		}
	})))
	o.update(func(sn *Snapshot) { sn.Proxy.SystemProxy = false })
	o.AddWarning(AppError{Code: CodeSysProxyTakenOver})
	o.log("proxy", CodeSysProxyTakenOver)
}

// RestoreProxyNow retries restoring the system proxy after
// SYSPROXY_RESTORE_FAILED, using the snapshot kept from that failure.
func (o *Orchestrator) RestoreProxyNow() error {
	o.mu.Lock()
	p := o.pending
	o.mu.Unlock()
	if o.d.SysProxy == nil || p == nil {
		return errors.New("no system proxy snapshot to restore")
	}
	if _, err := o.d.SysProxy.RestoreIfOurs(p.addr, p.snap); err != nil {
		return err
	}
	o.mu.Lock()
	o.pending = nil
	o.mu.Unlock()
	o.ClearWarning(CodeSysProxyRestore)
	return nil
}
