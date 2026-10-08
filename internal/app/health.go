package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

const (
	healthInterval = 30 * time.Second
	degradedAfter  = 15 * time.Second
	dpiSettleDelay = 2 * time.Second
	probeAttempts  = 2
)

// afterConnect starts background work once protected: DPI (if enabled),
// health checks and the post-connect site probe.
func (o *Orchestrator) afterConnect() {
	s := o.d.Settings()
	if s.DPI.Enabled {
		if err := o.startDPI(context.Background(), s); err != nil {
			o.logCodedErr("dpi", err)
		}
	}
	if o.d.Ticker != nil {
		ticks, stop := o.d.Ticker(healthInterval)
		ctx, cancel := context.WithCancel(context.Background())
		o.mu.Lock()
		o.healthStop = func() { cancel(); stop() }
		o.mu.Unlock()
		go o.healthLoop(ctx, ticks)
	}
	if o.d.Prober != nil {
		go o.probeBlocked(context.Background())
	}
	if st, ok := o.d.Picker.(interface{ Stale() bool }); ok && st.Stale() && o.refresh != nil {
		go o.refresh() // connected from an old ranking: rebuild it
	}
}

// ApplyBest switches a running connection to the best servers of the
// current ranking (after a full scan, or on another network).
func (o *Orchestrator) ApplyBest(ctx context.Context) {
	ctx, cancel := o.background(ctx) // Disconnect does not wait for it
	defer cancel()
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !o.connected() {
		return
	}
	picked, err := o.d.Picker.Pick(ctx, nil)
	if err != nil {
		slog.Warn("engine: picking the best servers failed", "err", err)
		return
	}
	o.mu.Lock()
	same := len(picked) == len(o.servers)
	for i := range picked {
		same = same && picked[i].ID == o.servers[i].ID
	}
	o.mu.Unlock()
	if !same {
		o.swapTo(ctx, picked)
	}
}

// logCodedErr logs err under its AppError code (CodeInternal otherwise),
// with its cause.
func (o *Orchestrator) logCodedErr(source string, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		o.logAppErr(source, ae)
		return
	}
	o.logErr(source, CodeInternal, err)
}

// upstreamsFailing reports whether every upstream in use has failed more
// recently than it last succeeded.
func (o *Orchestrator) upstreamsFailing(st engine.Stats) bool {
	if len(st.PerUpstream) == 0 {
		return false
	}
	for _, u := range st.PerUpstream {
		if !u.LastErrAt.After(u.LastOKAt) {
			return false
		}
	}
	return true
}

func (o *Orchestrator) healthLoop(ctx context.Context, ticks <-chan time.Time) {
	var failingSince time.Time
	for {
		var now time.Time
		select {
		case <-ctx.Done():
			return
		case now = <-ticks:
		}
		o.checkProxyHealth(ctx)
		o.checkDNSHealth(ctx)
		o.checkSNIHealth(ctx)
		failing := o.d.Engine.SelfTest(ctx) != nil || o.upstreamsFailing(o.d.Engine.Stats())
		if !failing {
			failingSince = time.Time{}
			o.clearReason(reasonUpstreams)
			continue
		}
		if failingSince.IsZero() {
			failingSince = now
			continue
		}
		if now.Sub(failingSince) >= degradedAfter {
			o.heal(ctx)
			failingSince = time.Time{}
		}
	}
}

// heal marks the connection degraded, picks fresh servers and hot-swaps
// them. DNS keeps pointing at loopback throughout, so nothing leaks.
func (o *Orchestrator) heal(ctx context.Context) {
	ctx, cancel := o.background(ctx)
	defer cancel()
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	o.addReason(reasonUpstreams)
	o.log("engine", "DEGRADED")
	var picked []model.Server
	var err error
	if fp, ok := o.d.Picker.(FreshPicker); ok {
		// Check again, skipping the servers that are failing now.
		o.mu.Lock()
		exclude := make([]string, 0, len(o.servers))
		for _, s := range o.servers {
			exclude = append(exclude, s.ID)
		}
		o.mu.Unlock()
		picked, err = fp.PickFresh(ctx, exclude)
	} else {
		picked, err = o.d.Picker.Pick(ctx, nil)
	}
	if err != nil {
		o.logErr("engine", CodeNoServers, err)
		return
	}
	o.swapTo(ctx, picked)
}

// swapTo hot-swaps the engine to picked. Callers hold opMu.
func (o *Orchestrator) swapTo(ctx context.Context, picked []model.Server) {
	ups, err := o.buildUpstreams(picked)
	if err != nil {
		slog.Warn("engine: building upstreams for a swap failed", "err", err)
		return
	}
	if err := o.d.Engine.Swap(ctx, ups); err != nil {
		// The engine may be gone while DNS still points at loopback: put
		// DNS back rather than stay "connected" to nothing.
		o.logErr("engine", "SWAP_FAILED", err)
		if errs := o.disconnectLocked(ctx); len(errs) > 0 {
			o.update(func(s *Snapshot) {
				s.Status, s.Error = StatusError, &AppError{Code: CodeRestoreFailed, Params: map[string]any{"adapter": errs[0].Target}}
			})
			return
		}
		o.update(func(s *Snapshot) {
			s.Status, s.Error, s.Servers, s.LatencyMs, s.Queries = StatusError, &AppError{Code: CodeEngineSelfTest}, nil, 0, 0
			s.DPI.Running, s.DPI.Engine, s.DPI.Fallback = false, "", false
		})
		return
	}
	o.mu.Lock()
	o.servers = picked
	o.mu.Unlock()
	o.update(func(s *Snapshot) { s.Servers = serverNames(picked) })
	o.clearReason(reasonUpstreams)
	o.log("ok", "SWAPPED", "servers", len(picked))
}

// OnNetworkChange lets the DNS backend catch up with the network: a new
// adapter is recorded and redirected (Windows), a configuration the
// system changed is set again (Linux). The updated snapshot is persisted
// before anything is applied.
func (o *Orchestrator) OnNetworkChange(ctx context.Context) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	s := o.d.Settings()

	o.mu.Lock()
	cur, v6 := o.dnsSnap, o.v6
	o.mu.Unlock()
	// A Reconcile error (the adapters could not be listed mid-flap) set
	// nothing: the next change tries again. It goes to the file log only.
	next, toApply, changes, err := o.d.DNS.Reconcile(cur, sysdns.Selection{Mode: s.Adapters, IDs: s.AdapterGUIDs})
	if err != nil {
		slog.Warn("system: reconciling DNS after a network change failed", "err", err)
	}
	if len(changes) > 0 {
		if err := o.d.States.Update(func(st *store.State) error {
			st.DNS = next
			return nil
		}); err != nil {
			slog.Warn("system: recording the updated DNS snapshot failed", "err", err)
		} else {
			o.mu.Lock()
			o.dnsSnap = next
			o.mu.Unlock()
			o.applyChanges(toApply, changes, v6)
		}
	}
	// Another network has its own ranking: use it, or build one.
	go func() {
		if st, ok := o.d.Picker.(interface{ Stale() bool }); ok && st.Stale() && o.refresh != nil {
			o.refresh()
			return
		}
		o.ApplyBest(context.Background())
	}()
}

// applyChanges applies what Reconcile found and logs each change: set, or
// failed. A backend that sets some targets and not others says which in
// an *sysdns.ApplyError; any other error fails them all.
func (o *Orchestrator) applyChanges(toApply sysdns.Snapshot, changes []sysdns.Change, v6 bool) {
	failed := map[string]bool{}
	if err := o.d.DNS.Apply(toApply, v6); err != nil {
		var ae *sysdns.ApplyError
		if !errors.As(err, &ae) {
			for _, c := range changes {
				o.log("system", CodeSetDNSFailed, "adapter", c.Target)
			}
			return
		}
		for _, t := range ae.Failed {
			failed[t] = true
		}
	}
	warnIgnored("dns flush", o.d.DNS.Flush())
	for _, c := range changes {
		code := CodeDNSReapplied
		switch {
		case failed[c.Target]:
			code = CodeSetDNSFailed
		case c.Added:
			code = "ADAPTER_ADDED"
		}
		o.log("system", code, "adapter", c.Target)
	}
}

// OnResume checks the engine after sleep and heals immediately on failure.
func (o *Orchestrator) OnResume(ctx context.Context) {
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	if o.d.Engine.SelfTest(ctx) != nil || o.upstreamsFailing(o.d.Engine.Stats()) {
		o.heal(ctx)
	}
}

// ProbeNow probes the configured sites once.
func (o *Orchestrator) ProbeNow(ctx context.Context) []probe.Result {
	if o.d.Prober == nil {
		return nil
	}
	return o.d.Prober.ProbeAll(ctx, o.d.Settings().ProbeSites)
}

func (o *Orchestrator) probeBlocked(ctx context.Context) {
	sites := o.d.Settings().ProbeSites
	results := make([][]probe.Result, probeAttempts)
	for i := range results {
		results[i] = o.d.Prober.ProbeAll(ctx, sites)
	}
	blocked := probe.DPIBlocked(results[0], results[1])
	o.update(func(s *Snapshot) {
		if s.Status == StatusProtected || s.Status == StatusDegraded {
			s.BlockedSites, s.Probed = blocked, true
		}
	})
	if len(blocked) == 0 {
		return
	}
	o.log("dpi", "SITES_BLOCKED", "count", len(blocked))
}

// FreshPicker is implemented by pickers that can ignore their cache and
// exclude servers (used when healing).
type FreshPicker interface {
	PickFresh(ctx context.Context, exclude []string) ([]model.Server, error)
}
