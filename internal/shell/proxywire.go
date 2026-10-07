package shell

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/proxy/dialer"
	"github.com/hashcott/ghostline/internal/proxy/wire"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/tlsfrag"
	"github.com/hashcott/ghostline/internal/winutil"
	builtinLists "github.com/hashcott/ghostline/lists"
)

// proxyWiring connects the proxy, rules, fragment cache and list
// downloads to the orchestrator and the UI service.
type proxyWiring struct {
	box    *app.SettingsBox
	eng    *engine.Engine
	paths  store.Paths
	exe    string
	fw     firewall.Manager
	bus    *app.Bus
	holder *rules.Holder
	frag   *store.FragCache
	log    *slog.Logger
	// mitm is the Fake SNI certificate source (phase S sets it).
	mitm mitmBox

	mu     sync.Mutex
	srv    *proxy.Server
	self   []netip.Addr
	selfAt time.Time
}

func newProxyWiring(box *app.SettingsBox, eng *engine.Engine, paths store.Paths, exe string, fw firewall.Manager, bus *app.Bus, log *slog.Logger) *proxyWiring {
	fc, err := store.LoadFragCache(paths.FragCache, time.Now())
	if err != nil {
		log.Warn("frag cache", "err", err)
	}
	return &proxyWiring{box: box, eng: eng, paths: paths, exe: exe, fw: fw, bus: bus, holder: &rules.Holder{}, frag: fc, log: log}
}

func (w *proxyWiring) fetcher() *lists.Fetcher {
	return &lists.Fetcher{Client: &http.Client{Timeout: 30 * time.Second}, Dir: w.paths.ListsDir, Now: time.Now,
		WriteFile: store.WriteFileAtomic, SigKey: serverListKey(), Fallback: builtinLists.FakeSNIFallback}
}

// fragCache adapts store.FragCache to the dialer for the current network.
type fragCache struct{ w *proxyWiring }

func (f fragCache) Has(host string) bool { return f.w.frag.Has(networkKey(), host, time.Now()) }
func (f fragCache) Add(host string) {
	days := f.w.box.Get().Proxy.Fragment.CacheDays
	f.w.frag.Add(networkKey(), host, time.Now().Add(time.Duration(days)*24*time.Hour))
	if err := f.w.frag.Save(f.w.paths.FragCache); err != nil {
		f.w.log.Warn("frag cache save", "err", err)
	}
}

func (w *proxyWiring) fragConfig() dialer.FragConfig {
	f := w.box.Get().Proxy.Fragment
	return dialer.FragConfig{Mode: f.Mode, Method: tlsfrag.Method(f.Method), Chunks: f.Chunks,
		Delay: time.Duration(f.DelayMs) * time.Millisecond, AutoTimeout: time.Duration(f.AutoTimeoutMs) * time.Millisecond}
}

// upstream returns an upstream with its password decrypted.
func (w *proxyWiring) upstream(id string) (dialer.Upstream, bool) {
	for _, u := range w.box.Get().Proxy.Upstreams {
		if u.ID != id {
			continue
		}
		out := dialer.Upstream{ID: u.ID, Type: u.Type, Addr: u.Addr, User: u.User}
		if u.PassEnc != "" {
			pass, err := winutil.UnprotectString(u.PassEnc)
			if err != nil {
				w.bus.Log(app.LogEvent{Time: time.Now(), Source: "proxy", Code: app.CodeUpstreamProxy, Params: map[string]any{"id": id}})
				return dialer.Upstream{}, false
			}
			out.Pass = pass
		}
		return out, true
	}
	return dialer.Upstream{}, false
}

// selfAddrs is every address of this machine (cached briefly: the dialer
// asks once per resolved address).
func (w *proxyWiring) selfAddrs() []netip.Addr {
	w.mu.Lock()
	defer w.mu.Unlock()
	if time.Since(w.selfAt) > 30*time.Second {
		w.self = append(winutil.LocalUnicastAddrs(), netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback())
		w.selfAt = time.Now()
	}
	return w.self
}

func (w *proxyWiring) newDialer(port int, matcher func() dialer.Matcher) *dialer.Dialer {
	lo4, lo6 := netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback()
	return dialer.New(dialer.Config{
		Resolver:  w.eng,
		Matcher:   matcher,
		Cache:     fragCache{w},
		Frag:      w.fragConfig,
		Upstreams: w.upstream,
		SelfAddrs: w.selfAddrs,
		Forbidden: []netip.AddrPort{
			netip.AddrPortFrom(lo4, uint16(port)), netip.AddrPortFrom(lo6, uint16(port)),
			netip.AddrPortFrom(lo4, 53), netip.AddrPortFrom(lo6, 53),
		},
	})
}

// Start implements app.Proxy.
func (w *proxyWiring) Start(ctx context.Context, run app.ProxyRun) error {
	port := w.box.Get().Proxy.Port
	srv := proxy.New(proxy.Config{
		Listen:   run.Listen,
		ShareLAN: run.ShareLAN,
		Dialer:   w.newDialer(port, func() dialer.Matcher { return w.holder.Load() }),
		OnConn:   w.bus.ProxyConn,
		MITM:     w.mitm.get,
	})
	if err := srv.Start(ctx); err != nil {
		return err
	}
	w.mu.Lock()
	w.srv = srv
	w.mu.Unlock()
	return nil
}

func (w *proxyWiring) server() *proxy.Server {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.srv
}

// Stop implements app.Proxy.
func (w *proxyWiring) Stop(ctx context.Context) error {
	w.mu.Lock()
	srv := w.srv
	w.srv = nil
	w.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Stop(ctx)
}

// SelfTest implements app.Proxy.
func (w *proxyWiring) SelfTest(ctx context.Context) error {
	if srv := w.server(); srv != nil {
		return srv.SelfTest(ctx)
	}
	return fmt.Errorf("proxy: not running")
}

// Alive implements app.Proxy.
func (w *proxyWiring) Alive() bool {
	srv := w.server()
	return srv != nil && srv.Alive()
}

// Stats implements app.ProxyQuery.
func (w *proxyWiring) Stats() proxy.Stats {
	if srv := w.server(); srv != nil {
		return srv.Stats()
	}
	return proxy.Stats{ByOutcome: map[string]uint64{}}
}

// lanInfo lists the addresses other devices can use to reach the proxy.
func (w *proxyWiring) lanInfo() app.LANInfo {
	port := strconv.Itoa(w.box.Get().Proxy.Port)
	info := app.LANInfo{Addrs: []string{}}
	for _, a := range winutil.LocalLANAddrs() {
		host := a.String()
		if a.Is6() {
			host = "[" + host + "]"
		}
		info.Addrs = append(info.Addrs, host+":"+port)
	}
	if pub, err := w.fw.IsPublicNetwork(); err == nil {
		info.Public = pub
	}
	return info
}

// upstreamMatcher sends everything through one upstream (connection test).
type upstreamMatcher string

func (m upstreamMatcher) Match(string, netip.Addr) rules.Decision {
	return rules.Decision{Action: rules.Action{Upstream: string(m)}}
}

// testUpstream completes a TLS handshake with www.google.com through id.
func (w *proxyWiring) testUpstream(ctx context.Context, id string) error {
	d := w.newDialer(w.box.Get().Proxy.Port, func() dialer.Matcher { return upstreamMatcher(id) })
	res, err := d.Open(ctx, netip.MustParseAddr("127.0.0.1"), wire.Target{Host: "www.google.com", Port: 443}, nil)
	if err != nil {
		return err
	}
	defer res.Conn.Close()
	return tls.Client(res.Conn, &tls.Config{ServerName: "www.google.com"}).HandshakeContext(ctx)
}

// runLists refreshes stale lists on the spec schedule until ctx ends.
func runLists(ctx context.Context, svc *app.Service) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := &lists.Scheduler{
		Lists:  func() []lists.List { return app.ListsForScheduler(svc) },
		Due:    lists.Due,
		Run:    func(ctx context.Context, id string) error { return app.RefreshListNow(svc, ctx, id) },
		Now:    time.Now,
		After:  time.After,
		Jitter: func() time.Duration { return time.Duration(r.Int63n(int64(10 * time.Minute))) },
	}
	s.Loop(ctx)
}

// runDNSServerStats emits dnsserver:stats and the setup page countdown
// every second while they are active.
func runDNSServerStats(ctx context.Context, eng *engine.Engine, orch *app.Orchestrator, bus *app.Bus, ticks <-chan time.Time) {
	wasOpen := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		if orch.Snapshot().DNSServer.Running {
			bus.Emit(app.EventDNSServerStats, eng.ServeStats())
		}
		url, left := orch.SetupInfo()
		if url != "" || wasOpen {
			bus.Emit(app.EventSetupCountdown, app.SetupCountdown{URL: url, RemainingSec: int(left.Seconds())})
		}
		wasOpen = url != ""
	}
}

// runProxyStats emits proxy:stats every second while the proxy runs.
func runProxyStats(ctx context.Context, w *proxyWiring, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		if w.Alive() {
			w.bus.Emit(app.EventProxyStats, w.Stats())
		}
	}
}
