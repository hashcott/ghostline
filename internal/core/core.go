// Package core builds Ghostline's logic: the orchestrator, the UI service,
// the event bus and the background loops. It has no window and no Wails:
// the Windows GUI runs it in-process, the Linux daemon serves it over RPC.
package core

import (
	"context"
	"log/slog"
	"math/rand"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/secrets"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/hashcott/ghostline/internal/upstreams"
	"github.com/hashcott/ghostline/internal/watchdog"
)

type (
	upstreamT = upstream.Upstream
	netipAddr = netip.Addr
)

// builderFunc adapts a function to app.Builder.
type builderFunc func(model.Server) (upstream.Upstream, error)

func (f builderFunc) Build(s model.Server) (upstream.Upstream, error) { return f(s) }

// Options carries what the host (GUI or daemon) provides.
type Options struct {
	Platform platform.Deps
	Log      *slog.Logger
	Emitter  app.Emitter // receives every UI event
	// Remote is true when the UI runs in other processes, for other users
	// (the Linux daemon, as root): lists may not name a local file.
	Remote bool
	// GUI hooks; nil in the daemon.
	SetMode           func(mode string)
	OnSettingsChanged func(old, n store.Settings) // after core's own handling
	OnUpdate          func(tag, url string)
	OpenFile          func(ctx context.Context, title string) (name string, data []byte, err error)
	SaveFile          func(ctx context.Context, name string, data []byte) error
}

// Core is a built Ghostline.
type Core struct {
	Orch     *app.Orchestrator
	Svc      *app.Service
	Bus      *app.Bus
	Settings *app.SettingsBox

	p       platform.Deps
	paths   store.Paths
	log     *slog.Logger
	eng     *engine.Engine
	pw      *proxyWiring
	cat     *catalog
	strats  *strategyBox
	checker *updateChecker
}

// New builds everything in the order the GUI always has: settings, state,
// the startup restore of a dead previous run, then the orchestrator and
// the service, then the rules.
func New(o Options) (*Core, error) {
	p, log := o.Platform, o.Log
	paths := p.Paths

	initial, settingsReset, err := store.LoadSettings(paths.Settings)
	if err != nil {
		log.Warn("settings", "err", err)
	}
	box := app.NewSettingsBox(paths.Settings, initial)

	states := store.NewStateStore(paths.State, p.Lock)
	strats := newStrategyBox(paths, serverListKey(), log)
	dpiMgr := NewDPIManager(paths, p, strats.get)
	nid := p.NetID
	recoverDeps := p.Recovery(states, dpiMgr.Stop, log)

	// Safety layer 3: restore whatever a dead previous run left behind.
	startOut, startErr := watchdog.RestoreIfOrphaned(recoverDeps)
	if startErr != nil {
		log.Error("startup restore", "err", startErr)
	} else if startOut != watchdog.NothingToDo {
		log.Info("startup restore", "outcome", startOut)
	}

	cat := newCatalog(paths)
	cache, _ := scanner.LoadCache(paths.ScanCache)
	factoryFor := func() (*upstreams.Factory, error) {
		s := box.Get()
		opts := upstreams.Options{Bootstrap: s.Bootstrap, Timeout: 3 * time.Second}
		if s.FragmentDNS.Enabled {
			opts.Fragment = &upstreams.FragmentOptions{Chunks: s.FragmentDNS.Chunks, Delay: time.Duration(s.FragmentDNS.DelayMs) * time.Millisecond}
		}
		return upstreams.NewFactory(opts)
	}
	build := builderFunc(func(sv model.Server) (upstreamT, error) {
		f, err := factoryFor()
		if err != nil {
			return nil, err
		}
		return f.Build(sv)
	})
	picker := &app.ScanPicker{
		Catalog: cat.get,
		Checker: scanner.DNSChecker{Build: build.Build, Domains: func() []string { return store.TestDomains(box.Get().TestDomain) }, Timeout: 3 * time.Second},
		Cache:   cache,
		SaveCache: func(c *scanner.Cache) error {
			return scanner.SaveCache(paths.ScanCache, c)
		},
		NetKey:   nid.NetworkKey,
		Settings: box.Get,
		Now:      time.Now,
		Rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	bus := app.NewBus(o.Emitter)
	// Every full scan (connect, level change, scan all) shows live in the UI.
	picker.Watch = func(done, total int, r *scanner.Result, running bool) {
		bus.Emit(app.EventScan, app.ScanProgress{Done: done, Total: total, Result: r, Running: running})
	}
	eng := engine.New(bus.Query)
	pw := newProxyWiring(box, eng, p, bus, log)
	cw := newCertWiring(p)
	dw := &dnsWiring{eng: eng, certs: cw}
	var svc *app.Service // assigned below; ConfirmOverride runs only after startup
	orch := app.New(app.Deps{
		Engine: eng, DNS: p.DNS, DPI: dpiMgr, Safety: safety{startup: p.Startup, startWatchdog: p.StartWatchdog}, System: system{procs: p.Procs},
		Picker: picker, Scans: picker, Builder: build, Resolver: net.DefaultResolver,
		Prober: probe.Prober{
			Resolve: func(ctx context.Context, host string) ([]netipAddr, error) {
				return net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
			},
			Dial: (&net.Dialer{}).DialContext, Timeout: 5 * time.Second,
		},
		Recover:      func() (watchdog.Outcome, error) { return watchdog.RestoreIfOrphaned(recoverDeps) },
		Sink:         bus,
		States:       states,
		Settings:     box.Get,
		SaveSettings: box.Save,
		Now:          time.Now,
		Sleep:        time.Sleep,
		Ticker: func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		},
		BlacklistPath:    paths.DPIBlacklist,
		AutoHostlistPath: paths.DPIAutoHostlist,
		Proxy:            pw,
		SysProxy:         sysproxy.Manager{API: p.SysProxy},
		Firewall:         p.Firewall,
		ConfirmOverride: func(ctx context.Context, server, pac string) bool {
			return svc != nil && app.AskOverride(ctx, svc, server, pac, 60*time.Second)
		},
		Rules: pw.holder.Load,

		DNSServer:    dw,
		Certs:        cw,
		LANAddrs:     netid.LocalLANAddrs,
		SetMITM:      pw.mitm.set,
		MITMSelfTest: pw.mitm.selfTest,
	})
	// A test domain most servers fail on is ignored; say so until it is fixed.
	picker.BrokenTestDomains = func(ds []string) {
		if len(ds) == 0 {
			orch.ClearWarning(app.CodeTestDomainBroken)
			return
		}
		orch.AddWarning(app.AppError{Code: app.CodeTestDomainBroken, Params: map[string]any{"domains": strings.Join(ds, ", ")}})
	}
	if settingsReset {
		orch.AddWarning(app.AppError{Code: app.CodeSettingsReset})
	}
	for _, w := range app.StartupWarnings(startOut, startErr) {
		orch.AddWarning(w)
	}

	fetcher := pw.fetcher()
	fetcher.NoFiles = o.Remote
	update := &updateState{}
	onUpdate := o.OnUpdate
	if onUpdate == nil {
		onUpdate = func(string, string) {}
	}
	checker := newUpdateChecker(&metaFile{path: paths.Meta}, update, bus, log, onUpdate)
	setMode := o.SetMode
	if setMode == nil {
		setMode = func(string) {}
	}
	svc = app.NewService(orch, app.ServiceDeps{
		Bus: bus, Paths: paths, Settings: box, Catalog: cat.get,
		LoadCustom: cat.loadCustom, SaveCustom: cat.saveCustom,
		ListAdapters: func() ([]sysdns.Adapter, error) { return p.DNS.Info().Adapters, nil },
		DNSInfo:      p.DNS.Info,
		StopService:  func(name string) error { return p.Procs.StopService(name, 10*time.Second) },
		SetMode:      setMode,
		RestoreNow:   func() error { return restoreNow(states, p.DNS) },
		Info: func() app.AppInfo {
			tag, url := update.get()
			return app.AppInfo{Version: brand.Version, Portable: paths.Portable, UpdateTag: tag, UpdateURL: url, Author: brand.Author, RepoURL: brand.RepoURL}
		},
		OnSettingsChanged: func(old, n store.Settings) {
			if old.StartWithWindows != n.StartWithWindows {
				err := p.Startup.SetAutostart(n.StartWithWindows)
				if err != nil {
					log.Error("autostart task", "err", err)
				}
			}
			if !slices.Equal(old.Bootstrap, n.Bootstrap) {
				picker.Checker = scanner.DNSChecker{Build: build.Build, Domains: func() []string { return store.TestDomains(box.Get().TestDomain) }, Timeout: 3 * time.Second}
			}
			if o.OnSettingsChanged != nil {
				o.OnSettingsChanged(old, n)
			}
		},
		LANDNSClients: func() int {
			if !orch.Snapshot().DNSServer.Running {
				return 0
			}
			return eng.ServeStats().Clients10m
		},
		Rules:           pw.holder,
		RulesPath:       paths.Rules,
		Fetcher:         fetcher,
		NoFileLists:     o.Remote,
		FragCache:       pw.frag,
		CheckTestDomain: func(d string) error { return picker.CheckDomain(context.Background(), d) },
		NetKey:          nid.NetworkKey,
		Proxy:           pw,
		LANInfo:         pw.lanInfo,
		Protect:         func(v string) (string, error) { return secrets.EncodeString(p.UserSecrets, v) },
		TestUpstream:    pw.testUpstream,
		CheckUpdate:     checker.checkNow,
		CheckServer: func(ctx context.Context, id string) error {
			_, err := picker.CheckOne(ctx, id)
			return err
		},
		NewSetupPage: func(files dnsserver.SetupFiles, onStop func()) app.SetupPage {
			p := dnsserver.NewSetupPage(files, time.Now)
			p.OnStop = onStop
			return p
		},
		CurrentSSID: func() string {
			ssid, err := nid.CurrentSSID()
			if err != nil {
				log.Warn("wifi name", "err", err)
			}
			return ssid
		},
		WifiNames: func() []string {
			names, err := nid.WifiNames()
			if err != nil {
				log.Warn("wifi names", "err", err)
			}
			return names
		},
		BuildUpstream: build.Build,
		PlainUpstream: plainUpstream,
		DialDirect:    dialDirect,
		ISPResolvers: func() []string {
			st, _ := states.Load()
			return ispResolvers(st, nid.LiveAdapters())
		},
		OpenFile: o.OpenFile,
		SaveFile: o.SaveFile,
	})
	if recovered, err := app.LoadRules(svc); err != nil || recovered {
		log.Warn("rules.json", "err", err, "recovered", recovered)
		orch.AddWarning(app.AppError{Code: app.CodeRulesParse, Params: map[string]any{"line": 0}})
	}
	return &Core{Orch: orch, Svc: svc, Bus: bus, Settings: box,
		p: p, paths: paths, log: log, eng: eng, pw: pw, cat: cat, strats: strats, checker: checker}, nil
}

// Start watches the network and the system proxy (in place when Start
// returns) and runs the background loops (stats, list refreshes, update
// checks) until ctx is done. wait returns once every loop has stopped and
// the watches are removed.
func (c *Core) Start(ctx context.Context) (wait func()) {
	var stops []func()
	if stop, err := c.p.WatchSysProxy(c.Orch.OnSysProxyChanged); err != nil {
		c.log.Warn("system proxy watch", "err", err)
	} else if stop != nil {
		stops = append(stops, stop)
	}
	if stop, err := c.p.DNS.Watch(func() { c.Orch.OnNetworkChange(context.Background()) }); err != nil {
		c.log.Warn("network watch", "err", err)
	} else if stop != nil {
		stops = append(stops, stop)
	}
	if c.p.WatchResume != nil {
		if stop, err := c.p.WatchResume(func() { go c.Orch.OnResume(context.Background()) }); err != nil {
			c.log.Warn("resume watch", "err", err)
		} else if stop != nil {
			stops = append(stops, stop)
		}
	}
	var wg sync.WaitGroup
	loop := func(f func()) {
		wg.Add(1)
		go func() { defer wg.Done(); f() }()
	}
	tick := func(f func(<-chan time.Time)) {
		loop(func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			f(t.C)
		})
	}
	tick(func(ticks <-chan time.Time) { app.RunStats(c.Svc, ctx, ticks) })
	tick(func(ticks <-chan time.Time) { runProxyStats(ctx, c.pw, ticks) })
	tick(func(ticks <-chan time.Time) { runDNSServerStats(ctx, c.eng, c.Orch, c.Bus, ticks) })
	loop(func() { runLists(ctx, c.Svc) })
	loop(func() { runUpdates(ctx, c.paths, c.Settings, c.cat, c.strats, c.checker, c.log) })
	return func() {
		wg.Wait()
		for _, stop := range stops {
			stop()
		}
	}
}

// Shutdown disconnects cleanly, putting the system back as it was.
func (c *Core) Shutdown(ctx context.Context) error { return c.Orch.Disconnect(ctx) }

// restoreNow is "Restore DNS now" when there is no connection to undo: it
// restores from state.json if a run left a snapshot, otherwise it undoes
// whatever Ghostline configuration the backend still finds.
func restoreNow(states *store.StateStore, dns sysdns.Backend) error {
	st, err := states.Load()
	if err == nil && st.Phase != store.PhaseClean && !st.DNS.Empty() {
		if errs := dns.Restore(st.DNS); len(errs) > 0 {
			return errs[0]
		}
		return states.Reset()
	}
	if err := dns.RestoreDefault(); err != nil {
		return err
	}
	return states.Reset()
}
