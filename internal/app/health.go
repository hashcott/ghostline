package app

import (
	"context"
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
			o.log("dpi", errCode(err))
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

func errCode(err error) string {
	if ae, ok := err.(*AppError); ok {
		return ae.Code
	}
	return CodeInternal
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
		o.log("engine", CodeNoServers)
		return
	}
	o.swapTo(ctx, picked)
}

// swapTo hot-swaps the engine to picked. Callers hold opMu.
func (o *Orchestrator) swapTo(ctx context.Context, picked []model.Server) {
	ups, err := o.buildUpstreams(picked)
	if err != nil {
		return
	}
	if err := o.d.Engine.Swap(ctx, ups); err != nil {
		// The engine may be gone while DNS still points at loopback: put
		// DNS back rather than stay "connected" to nothing.
		o.log("engine", "SWAP_FAILED")
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

// OnNetworkChange snapshots and redirects adapters that appeared while
// connected. The snapshot is persisted before the adapter is changed.
func (o *Orchestrator) OnNetworkChange(ctx context.Context) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	s := o.d.Settings()
	ads, err := o.d.DNS.Select(s.Adapters, s.AdapterGUIDs)
	if err != nil {
		return
	}
	o.mu.Lock()
	known := map[string]bool{}
	for _, sn := range o.snaps {
		known[sn.GUID] = true
	}
	o.mu.Unlock()
	for _, a := range ads {
		if known[a.GUID] {
			continue
		}
		snaps, err := o.d.DNS.Snapshot([]sysdns.Adapter{a})
		if err != nil || len(snaps) == 0 {
			continue
		}
		if err := o.d.States.Update(func(st *store.State) error {
			st.DNS.Windows = append(st.DNS.Windows, snaps...)
			return nil
		}); err != nil {
			continue
		}
		o.mu.Lock()
		o.snaps = append(o.snaps, snaps...)
		o.mu.Unlock()
		o.mu.Lock()
		v6 := o.v6
		o.mu.Unlock()
		if err := o.d.DNS.ApplyLoopback(snaps, v6); err != nil {
			o.log("system", CodeSetDNSFailed, "adapter", a.Alias)
			continue
		}
		_ = o.d.DNS.Flush()
		o.log("system", "ADAPTER_ADDED", "adapter", a.Alias)
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
