package app

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	builtinStrategies "github.com/hashcott/ghostline/assets/strategies"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/goodbyedpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/miekg/dns"
)

var errBoom = errors.New("boom")

// rec is a shared, ordered call log; fail makes the named call return errBoom.
type rec struct {
	mu    sync.Mutex
	calls []string
	fail  map[string]bool
}

func (r *rec) add(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, name)
	if r.fail[name] {
		return errBoom
	}
	return nil
}

func (r *rec) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type nopUp struct{}

func (nopUp) Exchange(context.Context, *dns.Msg) (*dns.Msg, error) { return nil, errBoom }
func (nopUp) Address() string                                      { return "nop" }
func (nopUp) Close() error                                         { return nil }

type fEngine struct {
	lastCfg engine.Config
	r       *rec
	saw     bool
	swaps   int
	stats   engine.Stats
	selfE   error
	mu      sync.Mutex
}

func (e *fEngine) Start(_ context.Context, cfg engine.Config) error {
	e.lastCfg = cfg
	return e.r.add("engine.start")
}
func (e *fEngine) Swap(context.Context, []upstream.Upstream) error {
	e.mu.Lock()
	e.swaps++
	e.mu.Unlock()
	return e.r.add("engine.swap")
}
func (e *fEngine) Stop(context.Context) error { return e.r.add("engine.stop") }
func (e *fEngine) SelfTest(context.Context) error {
	if err := e.r.add("engine.selftest"); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.selfE
}
func (e *fEngine) ExpectVerify(string) { _ = e.r.add("engine.expect") }
func (e *fEngine) SawVerify(string) bool {
	if e.r.add("engine.saw") != nil {
		return false
	}
	return e.saw
}
func (e *fEngine) Stats() engine.Stats { e.mu.Lock(); defer e.mu.Unlock(); return e.stats }

type fDNS struct {
	lastV6     bool
	r          *rec
	adapters   []sysdns.Adapter
	restoreErr bool
	// reapply makes Reconcile report these targets as set again (Linux).
	reapply []string
	// applyFail makes Apply fail on these adapters (by alias) and set the rest.
	applyFail    map[string]bool
	reconcileErr error
}

func (d *fDNS) Name() string { return "fake" }
func (d *fDNS) Snapshot(sysdns.Selection) (sysdns.Snapshot, error) {
	var out []model.AdapterSnapshot
	for _, a := range d.adapters {
		out = append(out, model.AdapterSnapshot{GUID: a.GUID, Alias: a.Alias, IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}})
	}
	if err := d.r.add("dns.snapshot"); err != nil {
		return sysdns.Snapshot{}, err
	}
	if len(out) == 0 {
		return sysdns.Snapshot{}, errors.New("no connected adapters")
	}
	return sysdns.Snapshot{Backend: "windows", Windows: out}, nil
}
func (d *fDNS) Apply(s sysdns.Snapshot, v6 bool) error {
	d.lastV6 = v6
	name := "dns.apply"
	if len(s.Windows) == 1 && s.Windows[0].GUID != "{A}" {
		name = "dns.apply:" + s.Windows[0].GUID
	}
	if err := d.r.add(name); err != nil {
		return err
	}
	var failed []string
	for _, a := range s.Windows {
		if d.applyFail[a.Alias] {
			failed = append(failed, a.Alias)
		}
	}
	if failed != nil {
		return &sysdns.ApplyError{Failed: failed, Err: errBoom}
	}
	return nil
}
func (d *fDNS) Reconcile(s sysdns.Snapshot, _ sysdns.Selection) (sysdns.Snapshot, sysdns.Snapshot, []sysdns.Change, error) {
	_ = d.r.add("dns.reconcile")
	if d.reconcileErr != nil {
		return s, sysdns.Snapshot{}, nil, d.reconcileErr
	}
	known := map[string]bool{}
	for _, a := range s.Windows {
		known[a.GUID] = true
	}
	next := s
	next.Windows = append([]model.AdapterSnapshot(nil), s.Windows...)
	toApply := sysdns.Snapshot{Backend: s.Backend}
	var changes []sysdns.Change
	for _, a := range d.adapters {
		if !known[a.GUID] {
			sn := model.AdapterSnapshot{GUID: a.GUID, Alias: a.Alias, IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}}
			next.Windows = append(next.Windows, sn)
			toApply.Windows = append(toApply.Windows, sn)
			changes = append(changes, sysdns.Change{Target: a.Alias, Added: true})
		}
	}
	for _, t := range d.reapply {
		toApply = next
		changes = append(changes, sysdns.Change{Target: t})
	}
	return next, toApply, changes, nil
}
func (d *fDNS) StillOurs(s sysdns.Snapshot) sysdns.Snapshot { return s }
func (d *fDNS) Restore(s sysdns.Snapshot) []sysdns.RestoreError {
	_ = d.r.add("dns.restore")
	if d.restoreErr {
		return []sysdns.RestoreError{{Target: s.Label(), Err: errBoom}}
	}
	return nil
}
func (d *fDNS) RestoreDefault() error        { return d.r.add("dns.restoredefault") }
func (d *fDNS) Flush() error                 { return d.r.add("dns.flush") }
func (d *fDNS) Watch(func()) (func(), error) { return func() {}, nil }
func (d *fDNS) Info() sysdns.Info {
	return sysdns.Info{Backend: "fake", AdapterPick: true, Adapters: d.adapters}
}

// fDPI runs no process but builds argv with the real engines, so tests can
// check what would be launched.
type fDPI struct {
	mu      sync.Mutex
	r       *rec
	running string           // engine ID, "" when stopped
	startE  error            // fails every start
	failOn  map[string]error // fails starts of one engine
	missing map[string]bool  // engines this "OS" does not have
	starts  []dpiStart
	onStart func()
}

type dpiStart struct {
	engine string
	plan   dpi.Plan
}

var testEngines = map[string]dpi.Engine{
	"goodbyedpi": goodbyedpi.New(),
	"zapret2": zapret2.New(func() strategies.List {
		l, err := strategies.Parse(builtinStrategies.BuiltinJSON, zapret2.ValidateArgs)
		if err != nil {
			panic(err)
		}
		return l
	}),
}

func (p *fDPI) Start(_ context.Context, engine string, plan dpi.Plan) (int, error) {
	_ = p.r.add("dpi.start")
	if p.onStart != nil {
		p.onStart()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.running = ""
	p.starts = append(p.starts, dpiStart{engine, plan})
	if p.startE != nil {
		return 0, p.startE
	}
	if err := p.failOn[engine]; err != nil {
		return 0, err
	}
	e, ok := testEngines[engine]
	if !ok {
		return 0, dpi.ErrUnknownEngine
	}
	if _, err := e.Args(plan); err != nil {
		return 0, err
	}
	p.running = engine
	return 99, nil
}
func (p *fDPI) Stop() error {
	p.setRunning("")
	return p.r.add("dpi.stop")
}
func (p *fDPI) Running() bool  { p.mu.Lock(); defer p.mu.Unlock(); return p.running != "" }
func (p *fDPI) Engine() string { p.mu.Lock(); defer p.mu.Unlock(); return p.running }
func (p *fDPI) RefreshLists(dpi.Plan) error {
	return p.r.add("dpi.refresh")
}
func (p *fDPI) Get(engine string) (dpi.Engine, bool) {
	if p.missing[engine] {
		return nil, false
	}
	e, ok := testEngines[engine]
	return e, ok
}
func (p *fDPI) setRunning(v string) {
	p.mu.Lock()
	p.running = v
	p.mu.Unlock()
}

// lastStart is the most recent Start call.
func (p *fDPI) lastStart() dpiStart {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.starts) == 0 {
		return dpiStart{}
	}
	return p.starts[len(p.starts)-1]
}

// startedArgs is the argv of the last start, with absolute list paths.
func (p *fDPI) startedArgs() []string {
	s := p.lastStart()
	if s.engine == "" {
		return nil
	}
	a, _ := testEngines[s.engine].Args(s.plan)
	return a
}

type fSafety struct{ r *rec }

func (s *fSafety) StartWatchdog(uint32, time.Time) (func() error, error) {
	if err := s.r.add("safety.watchdog"); err != nil {
		return nil, err
	}
	return func() error { return s.r.add("safety.watchdog.stop") }, nil
}
func (s *fSafety) CreateRecoveryTask() error { return s.r.add("safety.task.create") }
func (s *fSafety) DeleteRecoveryTask() error { return s.r.add("safety.task.delete") }

type fSystem struct {
	r           *rec
	admin       bool
	owners      []procs.PortOwner // port 53
	proxyOwners []procs.PortOwner // any other port
	noV6        bool
	listenErr   error            // returned by ListenFree
	probed      []netip.AddrPort // what ListenFree was asked to bind
}

func (s *fSystem) IsAdmin() bool { _ = s.r.add("sys.admin"); return s.admin }
func (s *fSystem) PortOwners(port uint16) ([]procs.PortOwner, error) {
	if port != 53 {
		return s.proxyOwners, nil
	}
	return s.owners, s.r.add("sys.ports")
}
func (s *fSystem) ListenFree(addrs []netip.AddrPort) error {
	s.probed = addrs
	if err := s.r.add("sys.listen"); err != nil {
		return err
	}
	return s.listenErr
}
func (s *fSystem) SelfPID() (uint32, time.Time) { return 1234, time.Unix(100, 0) }
func (s *fSystem) IPv6Available() bool          { return !s.noV6 }

type fPicker struct {
	r     *rec
	block chan struct{}
	err   error
	n     int
}

func (p *fPicker) Pick(ctx context.Context, _ func(done, total int)) ([]model.Server, error) {
	if err := p.r.add("pick"); err != nil {
		return nil, err
	}
	if p.block != nil {
		select {
		case <-p.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.err != nil {
		return nil, p.err
	}
	n := p.n
	if n == 0 {
		n = 1
	}
	var out []model.Server
	for i := 0; i < n; i++ {
		out = append(out, model.Server{ID: "cf", Name: "Cloudflare"})
	}
	return out, nil
}

type fBuilder struct{ r *rec }

func (b *fBuilder) Build(model.Server) (upstream.Upstream, error) { return nopUp{}, b.r.add("build") }

type fResolver struct{ r *rec }

func (f *fResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, f.r.add("resolve")
}

type fStates struct {
	r *rec
	s *store.StateStore
}

type memLock struct{ mu sync.Mutex }

func (l *memLock) Lock() error   { l.mu.Lock(); return nil }
func (l *memLock) Unlock() error { l.mu.Unlock(); return nil }

func (f *fStates) Load() (store.State, error) { return f.s.Load() }
func (f *fStates) Update(fn func(*store.State) error) error {
	var before store.State
	err := f.s.Update(func(st *store.State) error {
		before = *st
		if err := fn(st); err != nil {
			return err
		}
		switch {
		case st.Phase == store.PhaseClean:
			return f.r.add("state.clean")
		case before.Phase == store.PhaseDNSSet && len(st.DNS.Windows) > len(before.DNS.Windows):
			return f.r.add("state.append")
		default:
			return f.r.add("state.dns_set")
		}
	})
	return err
}

type fSink struct {
	mu     sync.Mutex
	states []Snapshot
	logs   []LogEvent
}

func (s *fSink) State(sn Snapshot) { s.mu.Lock(); s.states = append(s.states, sn); s.mu.Unlock() }
func (s *fSink) Log(e LogEvent)    { s.mu.Lock(); s.logs = append(s.logs, e); s.mu.Unlock() }

// snapshots copies the states so far: background work (the post-connect
// probe) may still be emitting.
func (s *fSink) snapshots() []Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Snapshot(nil), s.states...)
}

func (s *fSink) events() []LogEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]LogEvent(nil), s.logs...)
}

type fProber struct {
	r     *rec
	block bool // wait for ctx cancellation
	stage func(site string, call int) probe.Stage
	calls int
	mu    sync.Mutex
}

func (p *fProber) ProbeAll(ctx context.Context, sites []string) []probe.Result {
	p.mu.Lock()
	block := p.block
	p.mu.Unlock()
	if block {
		<-ctx.Done()
		return nil
	}
	p.mu.Lock()
	p.calls++
	call, stage := p.calls, p.stage
	p.mu.Unlock()
	var out []probe.Result
	for _, s := range sites {
		st := probe.StageOK
		if stage != nil {
			st = stage(s, call)
		}
		out = append(out, probe.Result{Site: s, Stage: st})
	}
	return out
}

type harness struct {
	o        *Orchestrator
	r        *rec
	eng      *fEngine
	dns      *fDNS
	dpi      *fDPI
	sys      *fSystem
	pick     *fPicker
	states   *fStates
	sink     *fSink
	prober   *fProber
	smu      sync.Mutex // guards settings against background readers
	settings store.Settings
	recovers int
}

func (h *harness) getSettings() store.Settings {
	h.smu.Lock()
	defer h.smu.Unlock()
	return h.settings
}

// setSettings changes settings safely while background work may read them.
func (h *harness) setSettings(fn func(s *store.Settings)) {
	h.smu.Lock()
	fn(&h.settings)
	h.smu.Unlock()
}

// setStage changes the outcome while background probes may be running
// (Connect starts one).
func (p *fProber) setStage(f func(site string, call int) probe.Stage) {
	p.mu.Lock()
	p.stage = f
	p.mu.Unlock()
}

func (p *fProber) setBlock(v bool) {
	p.mu.Lock()
	p.block = v
	p.mu.Unlock()
}

// goodbyeDefaults are the defaults with GoodbyeDPI, the engine most tests
// were written for; zapret2 tests switch explicitly.
func goodbyeDefaults() store.Settings {
	s := store.DefaultSettings()
	s.DPI.Engine = store.EngineGoodbyeDPI
	return s
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	r := &rec{fail: map[string]bool{}}
	h := &harness{r: r,
		eng:      &fEngine{r: r, saw: true},
		dns:      &fDNS{r: r, adapters: []sysdns.Adapter{{GUID: "{A}", Alias: "Wi-Fi", IfType: 71, Up: true, HasGateway: true}}},
		dpi:      &fDPI{r: r},
		sys:      &fSystem{r: r, admin: true},
		pick:     &fPicker{r: r},
		states:   &fStates{r: r, s: store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), &memLock{})},
		sink:     &fSink{},
		prober:   &fProber{r: r},
		settings: goodbyeDefaults(),
	}
	h.o = New(Deps{
		Engine: h.eng, DNS: h.dns, DPI: h.dpi, Safety: &fSafety{r: r}, System: h.sys, Picker: h.pick,
		Builder: &fBuilder{r: r}, Resolver: &fResolver{r: r},
		Recover: func() (watchdog.Outcome, error) { h.recovers++; _ = r.add("recover"); return watchdog.Restored, nil },
		Sink:    h.sink, States: h.states,
		Settings:     h.getSettings,
		SaveSettings: func(s store.Settings) error { h.smu.Lock(); h.settings = s; h.smu.Unlock(); return nil },
		Now:          time.Now,
		Prober:       h.prober,
		Sleep:        func(time.Duration) {},
	})
	return h
}
