package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

// Orchestrator owns the connection lifecycle.
type Orchestrator struct {
	d Deps

	opMu sync.Mutex // serialises connect/disconnect/swap/autotune

	mu           sync.Mutex
	snap         Snapshot
	cancel       context.CancelFunc
	dnsSnap      sysdns.Snapshot // what Connect changed, for Disconnect
	stopWatchdog func() error
	servers      []model.Server
	healthStop   func()
	// dirty: DNS may still point at loopback after a failed restore; the
	// engine, watchdog, recovery task and snapshot are kept until a restore
	// succeeds.
	dirty bool
	// refresh rebuilds the server ranking in the background and applies it
	// (set by the Service, which reports the scan's progress).
	refresh func()
	// v6 is whether the engine listens on [::1] for this connection.
	v6 bool
	// bgCtx is cancelled by Disconnect to stop autotune and healing.
	bgCtx    context.Context
	bgCancel context.CancelFunc
	// px is the proxy phase state; guarded by opMu.
	px proxyState
	// dns is the DNS server phase state; guarded by opMu.
	dns dnsState
	// sni is the Fake SNI phase state; guarded by opMu.
	sni sniState
	// blockPublic is whether the Public-profile block rule is in place;
	// guarded by opMu.
	blockPublic bool
	// sniTimer stops the pending debounced rotation; guarded by mu.
	sniTimer func() bool
	// setup is the open phone setup page; guarded by mu.
	setup    SetupPage
	setupURL string
	// pending is a system proxy restore that failed; guarded by mu.
	pending *pendingRestore
}

// New creates an orchestrator in the disconnected state.
func New(d Deps) *Orchestrator {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = time.Sleep
	}
	if d.AfterFunc == nil {
		d.AfterFunc = func(dur time.Duration, f func()) func() bool { return time.AfterFunc(dur, f).Stop }
	}
	if !d.ListenV4.IsValid() {
		d.ListenV4 = netip.MustParseAddrPort("127.0.0.1:53")
	}
	if !d.ListenV6.IsValid() {
		d.ListenV6 = netip.MustParseAddrPort("[::1]:53")
	}
	o := &Orchestrator{d: d, snap: Snapshot{Status: StatusDisconnected}}
	s := d.Settings()
	o.snap.DPI = DPIStatus{Enabled: s.DPI.Enabled, Preset: s.DPI.Preset}
	return o
}

// Snapshot returns a copy of the current UI state.
func (o *Orchestrator) Snapshot() Snapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snap.clone()
}

func (o *Orchestrator) emit() {
	o.mu.Lock()
	s := o.snap.clone()
	o.mu.Unlock()
	if o.d.Sink != nil {
		o.d.Sink.State(s)
	}
}

func (o *Orchestrator) update(fn func(*Snapshot)) {
	o.mu.Lock()
	fn(&o.snap)
	o.mu.Unlock()
	o.emit()
}

func (o *Orchestrator) log(source, code string, kv ...any) {
	if o.d.Sink == nil {
		return
	}
	e := LogEvent{Time: o.d.Now(), Source: source, Code: code}
	if len(kv) > 0 {
		e.Params = map[string]any{}
		for i := 0; i+1 < len(kv); i += 2 {
			e.Params[kv[i].(string)] = kv[i+1]
		}
	}
	o.d.Sink.Log(e)
}

// AddWarning adds a persistent warning (deduplicated by code and params).
func (o *Orchestrator) AddWarning(e AppError) {
	o.update(func(s *Snapshot) {
		for _, w := range s.Warnings {
			if w.Code == e.Code && sameParams(w.Params, e.Params) {
				return
			}
		}
		s.Warnings = append(s.Warnings, AppError{Code: e.Code, Params: e.Params})
	})
}

func sameParams(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// ClearWarning removes every warning with code.
func (o *Orchestrator) ClearWarning(code string) {
	o.update(func(s *Snapshot) {
		out := s.Warnings[:0]
		for _, w := range s.Warnings {
			if w.Code != code {
				out = append(out, w)
			}
		}
		s.Warnings = out
	})
}

// Cancel aborts an in-progress Connect.
func (o *Orchestrator) Cancel() {
	o.mu.Lock()
	c := o.cancel
	o.mu.Unlock()
	if c != nil {
		c()
	}
}

// Connect runs the connect saga (spec §5.2). Calling it while not
// disconnected (or in error) does nothing.
func (o *Orchestrator) Connect(ctx context.Context) error {
	o.mu.Lock()
	if o.snap.Status != StatusDisconnected && o.snap.Status != StatusError {
		o.mu.Unlock()
		return nil
	}
	cctx, cancel := context.WithCancel(ctx)
	o.cancel = cancel
	o.snap.Status, o.snap.Step, o.snap.Error, o.snap.BlockedSites, o.snap.Probed = StatusConnecting, 0, nil, nil, false
	o.mu.Unlock()
	o.emit()
	defer func() {
		cancel()
		o.mu.Lock()
		o.cancel = nil
		o.mu.Unlock()
	}()

	o.opMu.Lock()
	defer o.opMu.Unlock()

	err := runSteps(cctx, o.connectSteps(), func(i int) { o.update(func(s *Snapshot) { s.Step = i }) })
	if err != nil && errors.Is(err, errHalt) {
		// DNS could not be put back: keep engine, watchdog, recovery task
		// and the dns_set snapshot so Disconnect/RestoreNow/later layers retry.
		var ae *AppError
		errors.As(err, &ae)
		o.mu.Lock()
		o.dirty = true
		o.mu.Unlock()
		o.AddWarning(AppError{Code: CodeRestoreFailed, Params: ae.Params})
		o.update(func(s *Snapshot) { s.Status, s.Error = StatusError, &AppError{Code: ae.Code, Params: ae.Params} })
		o.log("system", ae.Code, flatten(ae.Params)...)
		return err
	}
	if err != nil {
		o.mu.Lock()
		o.dnsSnap, o.stopWatchdog = sysdns.Snapshot{}, nil
		o.mu.Unlock()
		if cctx.Err() != nil && ctx.Err() == nil || errors.Is(err, context.Canceled) {
			o.update(func(s *Snapshot) { s.Status, s.Step, s.Error = StatusDisconnected, 0, nil })
			o.log("system", "CONNECT_CANCELLED")
			return err
		}
		var ae *AppError
		if !errors.As(err, &ae) {
			ae = appErr(CodeInternal, err, "step", o.Snapshot().Step)
		}
		o.update(func(s *Snapshot) { s.Status, s.Error = StatusError, &AppError{Code: ae.Code, Params: ae.Params} })
		o.log("system", ae.Code, flatten(ae.Params)...)
		return err
	}
	o.update(func(s *Snapshot) {
		s.Status, s.Step, s.Since = StatusProtected, 0, o.d.Now()
		s.Servers = serverNames(o.servers)
	})
	o.log("ok", "CONNECTED", "servers", len(o.servers))
	_ = o.startProxyPhase(context.WithoutCancel(ctx))
	_ = o.startDNSPhase(context.WithoutCancel(ctx))
	_ = o.startSNIPhase(context.WithoutCancel(ctx))
	o.afterConnect()
	return nil
}

func flatten(m map[string]any) []any {
	var out []any
	for k, v := range m {
		out = append(out, k, v)
	}
	return out
}

// serverNames lists server names for the UI; names that appear more than
// once get the server's IP or host appended so they can be told apart.
func serverNames(ss []model.Server) []string {
	count := map[string]int{}
	for _, s := range ss {
		count[s.Name]++
	}
	out := make([]string, 0, len(ss))
	for i, s := range ss {
		if count[s.Name] < 2 {
			out = append(out, s.Name)
			continue
		}
		out = append(out, s.Name+" · "+serverDetail(s, i))
	}
	return out
}

// serverDetail is the server's first IP, its URL host, or its position.
func serverDetail(s model.Server, i int) string {
	if len(s.IPs) > 0 {
		return s.IPs[0]
	}
	if u, err := url.Parse(s.Address); err == nil && u.Hostname() != "" && u.Scheme != "sdns" {
		return u.Hostname()
	}
	return fmt.Sprintf("#%d", i+1)
}

func (o *Orchestrator) buildUpstreams(ss []model.Server) ([]upstream.Upstream, error) {
	var ups []upstream.Upstream
	for _, s := range ss {
		u, err := o.d.Builder.Build(s)
		if err != nil {
			for _, x := range ups {
				_ = x.Close()
			}
			return nil, err
		}
		ups = append(ups, u)
	}
	return ups, nil
}

func (o *Orchestrator) connectSteps() []step {
	var picked []model.Server
	var dnsSnap sysdns.Snapshot
	var stopWD func() error
	v6 := o.d.System.IPv6Available()
	return []step{
		{name: "preflight", do: func(ctx context.Context) error {
			if !o.d.System.IsAdmin() {
				return appErr(CodeNotAdmin, nil)
			}
			o.mu.Lock()
			dirty := o.dirty
			o.mu.Unlock()
			if dirty {
				// Our own earlier attempt left DNS on loopback. Restore from
				// that snapshot first; snapshotting now would record
				// 127.0.0.1 as the "original" DNS.
				if errs := o.disconnectLocked(ctx); len(errs) > 0 {
					return halt(appErr(CodeRestoreFailed, errs[0], "adapter", errs[0].Target))
				}
				o.ClearWarning(CodeRestoreFailed)
			}
			if st, err := o.d.States.Load(); err != nil || st.Phase != store.PhaseClean {
				if o.d.Recover != nil {
					if _, rerr := o.d.Recover(); rerr != nil {
						return appErr(CodeRestoreFailed, rerr)
					}
				}
			}
			// Try the bind rather than refuse whenever port 53 has an owner:
			// Mobile Hotspot holds 0.0.0.0:53, yet 127.0.0.1:53 still binds
			// and receives every loopback query.
			addrs := []netip.AddrPort{o.d.ListenV4}
			if v6 {
				addrs = append(addrs, o.d.ListenV6)
			}
			if lerr := o.d.System.ListenFree(addrs); lerr != nil {
				owners, err := o.d.System.PortOwners(53)
				if err != nil {
					return appErr(CodeInternal, err, "step", 1)
				}
				if len(owners) == 0 {
					return appErr(CodePort53Busy, lerr, "pid", 0, "name", "?", "service", "")
				}
				w := owners[0]
				return appErr(CodePort53Busy, lerr, "pid", w.PID, "name", w.Name, "service", w.Service)
			}
			return nil
		}},
		{name: "pick", do: func(ctx context.Context) error {
			ss, err := o.d.Picker.Pick(ctx, func(done, total int) {
				// A first scan checks every server: show how far it is,
				// without an event per server.
				if done == 1 || done == total || done%10 == 0 {
					o.update(func(s *Snapshot) { s.PickDone, s.PickTotal = done, total })
				}
			})
			o.update(func(s *Snapshot) { s.PickDone, s.PickTotal = 0, 0 })
			if err != nil {
				var np *NoPinnedError
				if errors.As(err, &np) {
					return appErr(CodeNoPinnedServers, err, "checked", np.Checked)
				}
				var ns *NoServersError
				if errors.As(err, &ns) {
					return appErr(CodeNoServers, err, "checked", ns.Checked, "elapsed", int64(ns.Elapsed/time.Second))
				}
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return appErr(CodeNoServers, err, "checked", 0, "elapsed", int64(0))
			}
			picked = ss
			return nil
		}},
		{name: "engine", do: func(ctx context.Context) error {
			ups, err := o.buildUpstreams(picked)
			if err != nil {
				return appErr(CodeEngineSelfTest, err)
			}
			cfg := engine.Config{ListenV4: o.d.ListenV4, Upstreams: ups, CacheEnabled: true,
				Rules: o.d.Rules, BlockMode: o.d.Settings().DNSBlockMode}
			if v6 {
				cfg.ListenV6 = o.d.ListenV6
			}
			if err := o.d.Engine.Start(ctx, cfg); err != nil {
				return appErr(CodeEngineSelfTest, err)
			}
			if err := o.d.Engine.SelfTest(ctx); err != nil {
				_ = o.d.Engine.Stop(context.WithoutCancel(ctx))
				return appErr(CodeEngineSelfTest, err)
			}
			o.mu.Lock()
			o.servers = picked
			o.v6 = v6
			o.mu.Unlock()
			return nil
		}, undo: func(ctx context.Context) error { return o.d.Engine.Stop(ctx) }},
		{name: "snapshot", do: func(ctx context.Context) error {
			s := o.d.Settings()
			snap, err := o.d.DNS.Snapshot(sysdns.Selection{Mode: s.Adapters, IDs: s.AdapterGUIDs})
			if err != nil {
				return appErr(CodeSetDNSFailed, err, "adapter", "")
			}
			dnsSnap = snap
			pid, start := o.d.System.SelfPID()
			if err := o.d.States.Update(func(st *store.State) error {
				st.Version, st.Phase, st.PID, st.PIDStartTime, st.StartedAt = 5, store.PhaseDNSSet, pid, start, o.d.Now()
				st.DNS = dnsSnap
				return nil
			}); err != nil {
				return err
			}
			o.mu.Lock()
			o.dnsSnap = dnsSnap
			o.mu.Unlock()
			return nil
		}, undo: func(ctx context.Context) error {
			return o.d.States.Update(func(st *store.State) error {
				*st = store.CleanState()
				return nil
			})
		}},
		{name: "safety", do: func(ctx context.Context) error {
			pid, start := o.d.System.SelfPID()
			stop, err := o.d.Safety.StartWatchdog(pid, start)
			if err != nil {
				return appErr(CodeInternal, err, "step", 5)
			}
			if err := o.d.Safety.CreateRecoveryTask(); err != nil {
				_ = stop()
				return appErr(CodeInternal, err, "step", 5)
			}
			stopWD = stop
			o.mu.Lock()
			o.stopWatchdog = stop
			o.mu.Unlock()
			return nil
		}, undo: func(ctx context.Context) error {
			err := o.d.Safety.DeleteRecoveryTask()
			if stopWD != nil {
				err = errors.Join(err, stopWD())
			}
			return err
		}},
		{name: "apply", do: func(ctx context.Context) error {
			// Apply can change part of the configuration before failing, so a
			// failure here restores before the rollback continues.
			if err := o.d.DNS.Apply(dnsSnap, v6); err != nil {
				return o.restoreOrHalt(dnsSnap, appErr(CodeSetDNSFailed, err, "adapter", dnsSnap.Label()))
			}
			if err := o.d.DNS.Flush(); err != nil {
				return o.restoreOrHalt(dnsSnap, appErr(CodeSetDNSFailed, err, "adapter", dnsSnap.Label()))
			}
			return nil
		}, undo: func(ctx context.Context) error {
			return o.restoreOrHalt(dnsSnap, nil)
		}},
		{name: "verify", do: func(ctx context.Context) error {
			nonce := randomHex(8)
			o.d.Engine.ExpectVerify(nonce)
			ips, err := o.d.Resolver.LookupNetIP(ctx, "ip4", nonce+".verify.ghostline.test")
			hit := false
			for _, ip := range ips {
				if ip == engine.VerifyAnswer {
					hit = true
				}
			}
			if err != nil || !hit || !o.d.Engine.SawVerify(nonce) {
				return appErr(CodeVerifyLeak, err)
			}
			return nil
		}},
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// restoreOrHalt puts snap back. On success it returns orig (the error that
// triggered the rollback, or nil); if any adapter cannot be restored it
// returns a halt error so the rollback keeps every safety layer.
func (o *Orchestrator) restoreOrHalt(snap sysdns.Snapshot, orig error) error {
	errs := o.d.DNS.Restore(snap)
	_ = o.d.DNS.Flush()
	if len(errs) > 0 {
		return halt(appErr(CodeRestoreFailed, errs[0], "adapter", errs[0].Target))
	}
	return orig
}

// disconnectLocked restores DNS and, only if that fully succeeds, stops
// GoodbyeDPI and the engine and disarms the safety net (spec §5.3). On a
// restore failure nothing else is touched: DNS still points at loopback, so
// the engine must keep answering and the watchdog/recovery task stay armed.
// Callers hold opMu.
func (o *Orchestrator) disconnectLocked(ctx context.Context) []sysdns.RestoreError {
	o.mu.Lock()
	dnsSnap, stopWD, healthStop := o.dnsSnap, o.stopWatchdog, o.healthStop
	o.mu.Unlock()

	// System proxy first, then firewall and proxy, then DNS (spec 2A 6.2).
	o.stopSNIPhase(ctx)
	o.stopDNSPhase(ctx)
	o.stopProxyPhase(ctx)
	o.dropBlockPublic()
	errs := o.d.DNS.Restore(dnsSnap)
	_ = o.d.DNS.Flush()
	if len(errs) > 0 {
		o.mu.Lock()
		o.dirty = true
		o.mu.Unlock()
		for _, e := range errs {
			o.AddWarning(AppError{Code: CodeRestoreFailed, Params: map[string]any{"adapter": e.Target}})
		}
		return errs
	}
	if healthStop != nil {
		healthStop()
	}
	if o.d.DPI.Running() {
		_ = o.d.DPI.Stop()
	}
	_ = o.d.Engine.Stop(ctx)
	_ = o.d.States.Update(func(s *store.State) error {
		*s = store.CleanState()
		// A session CA that could not be removed stays recorded so
		// recovery and "retry removal" still find it (spec 2B 6.5).
		if len(o.sni.installed) > 0 {
			s.Certs = &store.CertsState{Session: slices.Clone(o.sni.installed)}
		}
		return nil
	})
	if stopWD != nil {
		_ = stopWD()
	}
	_ = o.d.Safety.DeleteRecoveryTask()
	o.mu.Lock()
	o.dnsSnap, o.stopWatchdog, o.servers, o.healthStop, o.dirty = sysdns.Snapshot{}, nil, nil, nil, false
	o.mu.Unlock()
	o.ClearWarning(CodeRestoreFailed)
	return nil
}

// Disconnect restores DNS first, then stops GoodbyeDPI and the engine.
// If any adapter cannot be restored, the connection stays up (DNS still
// needs the engine) with a RESTORE_FAILED warning; calling Disconnect again
// retries.
func (o *Orchestrator) Disconnect(ctx context.Context) error {
	o.Cancel()
	o.cancelBackground() // stop autotune/heal so we do not wait for them
	o.opMu.Lock()
	defer o.opMu.Unlock()

	o.mu.Lock()
	prev := o.snap.Status
	connected := prev == StatusProtected || prev == StatusDegraded
	if !connected && !o.dirty {
		o.mu.Unlock()
		return nil
	}
	o.snap.Status = StatusDisconnecting
	o.mu.Unlock()
	o.emit()

	if errs := o.disconnectLocked(ctx); len(errs) > 0 {
		o.update(func(s *Snapshot) { s.Status = prev })
		o.log("system", CodeRestoreFailed, "adapter", errs[0].Target)
		return nil
	}
	o.update(func(s *Snapshot) {
		s.Status, s.Error, s.Since, s.Servers, s.BlockedSites, s.LatencyMs, s.Queries = StatusDisconnected, nil, time.Time{}, nil, nil, 0, 0
		s.DPI.Running, s.DPI.Engine, s.DPI.Fallback = false, "", false
		s.Reasons = nil
	})
	o.log("system", "DISCONNECTED")
	return nil
}

// background returns a context that Disconnect cancels, merged with ctx.
func (o *Orchestrator) background(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	o.mu.Lock()
	if o.bgCtx == nil {
		o.bgCtx, o.bgCancel = context.WithCancel(context.Background())
	}
	bg := o.bgCtx
	o.mu.Unlock()
	stop := context.AfterFunc(bg, cancel)
	return ctx, func() { stop(); cancel() }
}

// cancelBackground stops long-running background work and arms a fresh
// background context for later operations.
func (o *Orchestrator) cancelBackground() {
	o.mu.Lock()
	if o.bgCancel != nil {
		o.bgCancel()
	}
	o.bgCtx, o.bgCancel = context.WithCancel(context.Background())
	o.mu.Unlock()
}

// recordDPI persists GoodbyeDPI's state while DNS is redirected, so the
// watchdog and --restore also remove the WinDivert driver after a crash.
func (o *Orchestrator) recordDPI(running bool, pid int, engine string) {
	_ = o.d.States.Update(func(st *store.State) error {
		if st.Phase != store.PhaseDNSSet {
			return errNoChange
		}
		st.DPI = store.DPIState{Running: running, PID: pid, Engine: engine}
		return nil
	})
}

var errNoChange = errors.New("no change")
