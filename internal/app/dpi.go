package app

import (
	"context"
	"errors"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/store"
)

// dpiErr maps dpi errors to UI codes.
func dpiErr(err error, engine string) *AppError {
	code := CodeDPIStartFailed
	switch {
	case errors.Is(err, dpi.ErrHashMismatch):
		code = CodeDPIHashMismatch
	case errors.Is(err, dpi.ErrBlockedByAV):
		code = CodeDPIBlockedByAV
	case errors.Is(err, dpi.ErrKernelUnsupported):
		code = CodeDPIKernelUnsupported
	}
	return appErr(code, err, "engine", engine)
}

// planFor is what engine runs under settings s. A zapret2 strategy that is
// no longer in the list falls back to the first one.
func (o *Orchestrator) planFor(s store.Settings, engine string) dpi.Plan {
	p := dpi.Plan{Scope: dpi.Scope(s.DPI.Scope)}
	if p.Scope == dpi.ScopeBlacklist {
		p.Blacklist = o.d.BlacklistPath // the file may not exist with scope "all"
	}
	if engine != store.EngineZapret2 {
		p.Strategy, p.Custom = s.DPI.Preset, s.DPI.CustomArgs
		return p
	}
	p.Strategy, p.Custom = s.DPI.Zapret2.Strategy, s.DPI.Zapret2.CustomArgs
	if s.DPI.Zapret2.AutoHostlist {
		p.AutoHostlist = o.d.AutoHostlistPath
	}
	if p.Strategy != "custom" {
		if e, ok := o.d.DPI.Get(engine); ok {
			list := e.Strategies()
			known := false
			for _, st := range list {
				known = known || st.ID == p.Strategy
			}
			if !known && len(list) > 0 {
				o.log("dpi", "DPI_STRATEGY_RESET", "from", p.Strategy, "to", list[0].ID)
				p.Strategy = list[0].ID
			}
		}
	}
	return p
}

// startEngine starts one engine and records it in state.json.
func (o *Orchestrator) startEngine(ctx context.Context, engine string, p dpi.Plan) error {
	pid, err := o.d.DPI.Start(ctx, engine, p)
	if err != nil {
		return dpiErr(err, engine)
	}
	o.recordDPI(true, pid, engine)
	return nil
}

// startDPI starts the configured engine. A zapret2 that is blocked or
// tampered with is replaced by GoodbyeDPI for this run only: the settings
// keep zapret2, so the next start tries it again (spec §8.1).
func (o *Orchestrator) startDPI(ctx context.Context, s store.Settings) error {
	engine := s.DPI.Engine
	if _, ok := o.d.DPI.Get(engine); !ok {
		// Settings from another OS (a Windows backup naming GoodbyeDPI).
		engine = store.EngineZapret2
	}
	p := o.planFor(s, engine)
	err := o.startEngine(ctx, engine, p)
	if err == nil {
		o.update(func(sn *Snapshot) {
			sn.DPI = DPIStatus{Enabled: true, Running: true, Engine: engine, Preset: p.Strategy}
		})
		o.clearReason(ReasonDPIFallback)
		o.log("dpi", "DPI_STARTED", "engine", engine, "preset", p.Strategy)
		return nil
	}
	first := err.(*AppError)
	if engine != store.EngineZapret2 || (first.Code != CodeDPIBlockedByAV && first.Code != CodeDPIHashMismatch) {
		return first
	}
	fb := o.planFor(s, store.EngineGoodbyeDPI)
	if o.startEngine(ctx, store.EngineGoodbyeDPI, fb) != nil {
		return first
	}
	o.update(func(sn *Snapshot) {
		sn.DPI = DPIStatus{Enabled: true, Running: true, Engine: store.EngineGoodbyeDPI, Preset: fb.Strategy, Fallback: true}
	})
	o.addReason(ReasonDPIFallback)
	o.log("dpi", CodeDPIFallback, "from", store.EngineZapret2, "to", store.EngineGoodbyeDPI, "cause", first.Code)
	return nil
}

// stopDPI stops the engine and forgets a fallback. The snapshot says so
// at once: a restart takes seconds and the UI must not keep showing the
// engine being stopped.
func (o *Orchestrator) stopDPI() {
	_ = o.d.DPI.Stop()
	o.recordDPI(false, 0, "")
	o.update(func(sn *Snapshot) {
		sn.DPI.Running, sn.DPI.Engine, sn.DPI.Preset, sn.DPI.Fallback = false, "", "", false
	})
	o.clearReason(ReasonDPIFallback)
}

func (o *Orchestrator) connected() bool {
	st := o.Snapshot().Status
	return st == StatusProtected || st == StatusDegraded
}

// RestartDPI restarts a running engine so new options or a new blacklist
// take effect, with the configured engine (so a fallback retries zapret2).
// It does nothing when no engine is running.
func (o *Orchestrator) RestartDPI(ctx context.Context) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if !o.d.DPI.Running() {
		return nil
	}
	o.stopDPI()
	if err := o.startDPI(ctx, o.d.Settings()); err != nil {
		o.update(func(sn *Snapshot) { sn.DPI.Running, sn.DPI.Engine, sn.DPI.Fallback = false, "", false })
		return err
	}
	return nil
}

// RewriteZapret2File runs write while a running zapret2 is stopped (it keeps
// its own copy of the auto-detected list and would write it back), then
// starts DPI again if it was running and Ghostline is still connected.
func (o *Orchestrator) RewriteZapret2File(ctx context.Context, write func() error) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	stopped := o.d.DPI.Engine() == store.EngineZapret2
	if stopped {
		o.stopDPI() // copies the engine's list out first
	}
	err := write()
	if !stopped || !o.connected() {
		if stopped {
			o.update(func(sn *Snapshot) { sn.DPI.Running, sn.DPI.Engine, sn.DPI.Fallback = false, "", false })
		}
		return err
	}
	if serr := o.startDPI(ctx, o.d.Settings()); serr != nil {
		o.update(func(sn *Snapshot) { sn.DPI.Running, sn.DPI.Engine, sn.DPI.Fallback = false, "", false })
		return serr
	}
	return err
}

// RefreshDPILists hands a new blacklist to a running engine that re-reads
// it by itself, and restarts any other engine.
func (o *Orchestrator) RefreshDPILists(ctx context.Context) error {
	if e, ok := o.d.DPI.Get(o.d.DPI.Engine()); ok && e.HotReloadsLists() {
		return o.d.DPI.RefreshLists(o.planFor(o.d.Settings(), e.ID()))
	}
	return o.RestartDPI(ctx)
}

// SetDPIEnabled turns the DPI engine on or off. While disconnected it only
// saves the setting; enabled DPI starts on the next connect.
func (o *Orchestrator) SetDPIEnabled(ctx context.Context, on bool) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	s := o.d.Settings()
	if on && o.connected() && !o.d.DPI.Running() {
		// Starting takes seconds; say "enabled" now so snapshots emitted
		// meanwhile don't flip the UI switch back.
		o.update(func(sn *Snapshot) { sn.DPI.Enabled = true })
		if err := o.startDPI(ctx, s); err != nil {
			o.update(func(sn *Snapshot) { sn.DPI.Enabled = s.DPI.Enabled })
			return err
		}
	}
	if !on && o.d.DPI.Running() {
		o.stopDPI()
		o.log("dpi", "DPI_STOPPED")
	}
	s.DPI.Enabled = on
	if err := o.d.SaveSettings(s); err != nil {
		return err
	}
	o.update(func(sn *Snapshot) {
		sn.DPI.Enabled = on
		sn.DPI.Running = o.d.DPI.Running()
		if !sn.DPI.Running {
			sn.DPI.Engine, sn.DPI.Fallback = "", false
		}
	})
	return nil
}
