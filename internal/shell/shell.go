// Package shell wires Ghostline together and owns the Wails window and tray.
package shell

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashcott/ghostline/internal/platform"
	"io/fs"
	"log/slog"
	"math/rand"
	"net"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/secrets"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/hashcott/ghostline/internal/upstreams"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Options carries what main provides.
type Options struct {
	Mode       cli.Mode
	Assets     fs.FS
	Executable string
	// Platform is this OS's wiring (platform.New).
	Platform platform.Deps
}

// Fatal reports an error that stops Ghostline before its window exists.
func Fatal(err error) { fatalBox(err) }

// Run starts the UI process.
func Run(o Options) error {
	if err := preflight(); err != nil {
		return err
	}
	p := o.Platform
	paths := p.Paths
	if err := os.MkdirAll(paths.DataDir, 0o755); err != nil {
		fatalBox(err)
		return err
	}
	logw, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3)
	if err != nil {
		fatalBox(err)
		return err
	}
	defer logw.Close()
	log := slog.New(slog.NewTextHandler(logw, nil))
	slog.SetDefault(log)
	log.Info("start", "version", brand.Version, "portable", paths.Portable, "mode", o.Mode.Kind)

	initial, settingsReset, err := store.LoadSettings(paths.Settings)
	if err != nil {
		log.Warn("settings", "err", err)
	}
	box := app.NewSettingsBox(paths.Settings, initial)

	states := store.NewStateStore(paths.State, p.Lock)
	dnsMgr := sysdns.NewManager(p.DNS, time.Sleep)
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

	var wapp *application.App
	em := &emitter{}
	bus := app.NewBus(em)
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
		Engine: eng, DNS: dnsMgr, DPI: dpiMgr, Safety: safety{startup: p.Startup, startWatchdog: p.StartWatchdog}, System: system{procs: p.Procs},
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

	ui := &ui{orch: orch, box: box, log: log, lanDNSClients: func() int {
		if !orch.Snapshot().DNSServer.Running {
			return 0
		}
		return eng.ServeStats().Clients10m
	}}
	update := &updateState{}
	checker := newUpdateChecker(&metaFile{path: paths.Meta}, update, bus, log, func(tag, url string) { ui.onUpdate(tag, url) })
	ui.checker = checker
	svc = app.NewService(orch, app.ServiceDeps{
		Bus: bus, Paths: paths, Settings: box, Catalog: cat.get,
		LoadCustom: cat.loadCustom, SaveCustom: cat.saveCustom,
		ListAdapters: p.DNS.Adapters,
		StopService:  func(name string) error { return p.Procs.StopService(name, 10*time.Second) },
		SetMode:      ui.setMode,
		RestoreNow:   func() error { return restoreNow(states, dnsMgr) },
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
			if old.Language != n.Language {
				ui.onLanguage()
			}
			if !slices.Equal(old.Bootstrap, n.Bootstrap) {
				picker.Checker = scanner.DNSChecker{Build: build.Build, Domains: func() []string { return store.TestDomains(box.Get().TestDomain) }, Timeout: 3 * time.Second}
			}
			if old.Proxy.Enabled != n.Proxy.Enabled {
				ui.onLanguage() // relabels the tray's proxy item
			}
		},
		Rules:           pw.holder,
		RulesPath:       paths.Rules,
		Fetcher:         pw.fetcher(),
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
		OpenFile: func(title string) (string, error) {
			if wapp == nil {
				return "", errors.New("no window")
			}
			return wapp.Dialog.OpenFile().SetTitle(title).AddFilter("Ghostline backup (*.json)", "*.json").PromptForSingleSelection()
		},
		SaveFile: func(name string, data []byte) error {
			if wapp == nil {
				return errors.New("no window")
			}
			path, err := wapp.Dialog.SaveFile().SetFilename(name).PromptForSingleSelection()
			if err != nil || path == "" {
				return err // cancelled: nothing to save
			}
			return os.WriteFile(path, data, 0o644)
		},
	})
	ui.svc = svc
	if recovered, err := app.LoadRules(svc); err != nil || recovered {
		log.Warn("rules.json", "err", err, "recovered", recovered)
		orch.AddWarning(app.AppError{Code: app.CodeRulesParse, Params: map[string]any{"line": 0}})
	}

	opts := application.Options{
		Name:        brand.AppName,
		Description: "Secure DNS client",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(o.Assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               brand.SingleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { ui.show() },
		},
		OnShutdown: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = orch.Disconnect(ctx)
		},
	}
	applyPlatformOptions(&opts, orch)
	wapp = application.New(opts)
	em.app = wapp
	ui.app = wapp
	if o.Mode.Kind != cli.KindAutostart {
		// Started with Windows: stay in the tray without a window, so no
		// WebView2 runs until the user opens Ghostline.
		ui.createWindow()
	}
	ui.createTray()
	em.onState = ui.onState

	if stopProxyWatch, err := p.WatchSysProxy(orch.OnSysProxyChanged); err != nil {
		log.Warn("system proxy watch", "err", err)
	} else {
		defer stopProxyWatch()
	}
	stopWatch, err := p.WatchNetwork(func() { orch.OnNetworkChange(context.Background()) })
	if err != nil {
		log.Warn("network watch", "err", err)
	} else {
		defer stopWatch()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	statTick := time.NewTicker(time.Second)
	defer statTick.Stop()
	go app.RunStats(svc, ctx, statTick.C)
	proxyTick := time.NewTicker(time.Second)
	defer proxyTick.Stop()
	go runProxyStats(ctx, pw, proxyTick.C)
	dnsTick := time.NewTicker(time.Second)
	defer dnsTick.Stop()
	go runDNSServerStats(ctx, eng, orch, bus, dnsTick.C)
	go runLists(ctx, svc)
	go runUpdates(ctx, paths, box, cat, strats, checker, log)

	if o.Mode.Kind == cli.KindAutostart && box.Get().AutoConnect {
		go func() { _ = orch.Connect(context.Background()) }()
	}
	if err := wapp.Run(); err != nil {
		fatalBox(err)
		return fmt.Errorf("shell: %w", err)
	}
	return nil
}

// restoreNow puts DNS back from state.json, or resets loopback adapters to
// DHCP when there is no usable snapshot.
func restoreNow(states *store.StateStore, mgr *sysdns.Manager) error {
	st, err := states.Load()
	if err == nil && len(st.Snapshot) > 0 {
		if errs := mgr.Restore(st.Snapshot); len(errs) > 0 {
			return errs[0]
		}
		return states.Reset()
	}
	ads, err := mgr.LoopbackAdapters()
	if err != nil {
		return err
	}
	var snaps []model.AdapterSnapshot
	for _, a := range ads {
		snaps = append(snaps, model.AdapterSnapshot{GUID: a.GUID, IfIndex: a.IfIndex, Alias: a.Alias,
			IPv4: model.FamilyDNS{Mode: model.DNSModeDHCP}, IPv6: model.FamilyDNS{Mode: model.DNSModeDHCP}})
	}
	if errs := mgr.Restore(snaps); len(errs) > 0 {
		return errs[0]
	}
	return states.Reset()
}
