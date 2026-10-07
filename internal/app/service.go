package app

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
)

// SettingsBox holds the live settings and persists every change.
type SettingsBox struct {
	mu   sync.Mutex
	path string
	s    store.Settings
}

// NewSettingsBox wraps initial settings stored at path.
func NewSettingsBox(path string, initial store.Settings) *SettingsBox {
	return &SettingsBox{path: path, s: initial}
}

// Get returns the current settings.
func (b *SettingsBox) Get() store.Settings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.s
}

// Save persists and adopts s.
// set replaces the settings in memory only (the file was already written).
func (b *SettingsBox) set(s store.Settings) {
	b.mu.Lock()
	b.s = s
	b.mu.Unlock()
}

func (b *SettingsBox) Save(s store.Settings) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := store.SaveSettings(b.path, s); err != nil {
		return err
	}
	b.s = s
	return nil
}

// AppInfo describes the running build.
type AppInfo struct {
	Version   string `json:"version"`
	Portable  bool   `json:"portable"`
	UpdateTag string `json:"updateTag"`
	UpdateURL string `json:"updateUrl"`
	Author    string `json:"author"`
	RepoURL   string `json:"repoUrl"`
}

// ServerRow is one line of the Servers page.
type ServerRow struct {
	Server model.Server    `json:"server"`
	Result *scanner.Result `json:"result,omitempty"`
	InUse  bool            `json:"inUse"`
	Pinned bool            `json:"pinned"`
	// Auto: the settings let it be picked automatically (no-filter tags,
	// custom or pinned).
	Auto bool `json:"auto"`
}

// ServiceDeps wires the UI service.
type ServiceDeps struct {
	Bus          *Bus
	Paths        store.Paths
	Settings     *SettingsBox
	Catalog      func() []model.Server
	LoadCustom   func() ([]model.Server, error)
	SaveCustom   func([]model.Server) error
	ListAdapters func() ([]sysdns.Adapter, error)
	DNSInfo      func() sysdns.Info // the system DNS backend, for Settings
	StopService  func(name string) error
	SetMode      func(mode string)
	RestoreNow   func() error
	Info         func() AppInfo
	// OnSettingsChanged lets the shell react (autostart task, language…).
	OnSettingsChanged func(old, new store.Settings)

	// Phase 2A.
	Rules      *rules.Holder
	RulesPath  string
	Fetcher    *lists.Fetcher
	MaxEntries int // 0 = DefaultMaxEntries
	FragCache  *store.FragCache
	NetKey     func() string
	// CheckTestDomain, when set, checks that a new test domain has an IPv4
	// address before it is saved.
	CheckTestDomain func(domain string) error
	Proxy           ProxyQuery
	LANInfo         func() LANInfo
	Protect         func(string) (string, error) // DPAPI
	TestUpstream    func(ctx context.Context, id string) error
	// CheckUpdate asks GitHub for the latest release now (manual check).
	CheckUpdate func(ctx context.Context) (UpdateCheck, error)
	// CheckServer re-tests one server and updates the cached scan.
	CheckServer func(ctx context.Context, id string) error

	// Phase 2B.
	NewSetupPage func(files dnsserver.SetupFiles, onStop func()) SetupPage
	CurrentSSID  func() string
	// WifiNames lists Wi-Fi networks this PC knows (saved and in range).
	WifiNames func() []string
	// SaveFile asks where to save data (native dialog) and writes it. ctx
	// is the bound call's: the Linux daemon uses it to reach the GUI that
	// asked.
	SaveFile func(ctx context.Context, name string, data []byte) error
	// NoFileLists refuses lists from a local file: the service runs as
	// root for clients in other processes (the Linux daemon).
	NoFileLists bool
	// LANDNSClients counts LAN devices that used the DNS server in the last
	// 10 minutes (0 while it is off).
	LANDNSClients func() int

	// Phase 3 tools.
	BuildUpstream func(model.Server) (upstream.Upstream, error)
	PlainUpstream func(ip string) (upstream.Upstream, error)                        // plain UDP 53: Ghostline's own engine and the ISP lookup source only
	DialDirect    func(ctx context.Context, network, addr string) (net.Conn, error) // clean-IP scan: straight out, not via the proxy
	// OpenFile asks for a file (native dialog) and returns its name and
	// contents, read by the GUI; name "" when cancelled. Ghostline never
	// opens a path the user picked with its own privileges.
	OpenFile     func(ctx context.Context, title string) (name string, data []byte, err error)
	ISPResolvers func() []string // this PC's DNS before Ghostline took over
}

// Service is bound to the frontend by Wails; its exported methods are the
// UI's whole API.
type Service struct {
	o *Orchestrator
	x ServiceDeps

	mu         sync.Mutex
	scanCancel context.CancelFunc

	// Phase 3 tools: one job per tool (guarded by mu).
	advCancel  context.CancelFunc
	adv        advJob
	cfCancel   context.CancelFunc
	cf         cfJob
	cfCacheMu  sync.Mutex       // load-modify-save of cfscan-cache.json
	cfRoots    *x509.CertPool   // nil = system roots; tests inject a test CA
	imp        *importTicket    // the last import preview
	now        func() time.Time // nil = time.Now; tests inject
	tuneCancel context.CancelFunc
	overrideCh chan bool

	rmu sync.Mutex // guards rf
	rf  store.RulesFile
	bg  sync.WaitGroup
}

// NewService creates the UI service.
func NewService(o *Orchestrator, x ServiceDeps) *Service {
	s := &Service{o: o, x: x}
	o.refresh = func() { _ = s.ScanAll() }
	return s
}

// GetSnapshot returns the current state.
func (s *Service) GetSnapshot() Snapshot { return s.o.Snapshot() }

// Connect starts protection. Errors are also reported in the snapshot.
func (s *Service) Connect() error {
	slog.Info("ui: connect requested")
	return s.o.Connect(context.Background())
}

// Disconnect stops protection.
func (s *Service) Disconnect() error { return s.o.Disconnect(context.Background()) }

// CancelConnect aborts a connect in progress.
func (s *Service) CancelConnect() { s.o.Cancel() }

// GetSettings returns the settings.
func (s *Service) GetSettings() store.Settings { return s.x.Settings.Get() }

// SaveSettings validates and stores settings.
func (s *Service) SaveSettings(n store.Settings) error { return s.saveSettings(n, false) }

// saveSettings validates and stores n. The DNS server and Fake SNI blocks
// change only through their own bindings (owned=true): the UI's copy of the
// settings may be stale and must not undo them.
// validateSettings checks the fields every save (and every import) must
// pass.
func (s *Service) validateSettings(n store.Settings) error {
	if n.Language != "vi" && n.Language != "en" {
		return fmt.Errorf("settings: language must be vi or en")
	}
	if n.MaxUpstreams < 1 || n.MaxUpstreams > 10 {
		return fmt.Errorf("settings: maxUpstreams must be 1..10")
	}
	if err := store.ValidateTestDomains(n.TestDomain); err != nil {
		return err
	}
	if len(n.Bootstrap) == 0 {
		return fmt.Errorf("settings: bootstrap list is empty")
	}
	for _, b := range n.Bootstrap {
		if _, err := netip.ParseAddrPort(b); err != nil {
			return fmt.Errorf("settings: bootstrap %q must be ip:port", b)
		}
	}
	if err := store.ValidateProxy(n.Proxy); err != nil {
		return err
	}
	if err := store.ValidateTools(n.Tools); err != nil {
		return err
	}
	if n.DNSBlockMode != "zero" && n.DNSBlockMode != "nxdomain" {
		return fmt.Errorf("settings: dnsBlockMode must be zero or nxdomain")
	}
	if err := s.validateDPI(n.DPI); err != nil {
		return err
	}
	return nil
}

func (s *Service) saveSettings(n store.Settings, owned bool) error {
	n.TestDomain = strings.Join(store.TestDomains(n.TestDomain), "\n")
	if err := s.validateSettings(n); err != nil {
		return err
	}
	old := s.x.Settings.Get()
	if s.x.CheckTestDomain != nil {
		was := store.TestDomains(old.TestDomain)
		for _, d := range store.TestDomains(n.TestDomain) {
			if !slices.Contains(was, d) && s.x.CheckTestDomain(d) != nil {
				return appErr(CodeTestDomainNoAddress, errors.New(d))
			}
		}
	}
	if !owned {
		n.DNSServer, n.FakeSNI = old.DNSServer, old.FakeSNI
	}
	if err := store.ValidateDNSServer(n.DNSServer, n.Proxy.Port); err != nil {
		return err
	}
	if n.FakeSNI.Enabled && n.FakeSNI.AckVersion < FakeSNIWarningVersion {
		return appErr(CodeFakeSNINotAcked, nil)
	}
	if n.DPI.Scope == string(dpi.ScopeBlacklist) && old.DPI.Scope != n.DPI.Scope {
		if txt, _ := s.GetDPIBlacklist(); dpi.BlacklistEntries(txt) == 0 {
			return appErr(CodeDPIBlacklistEmpty, nil)
		}
	}
	// Upstream proxies change only through SaveUpstreamProxy/
	// DeleteUpstreamProxy: the UI's copy may be stale or carry masked
	// passwords, so it never overwrites them. Pins likewise change only
	// through SetPinned/SetPinnedMany.
	n.Proxy.Upstreams = old.Proxy.Upstreams
	n.Pinned = old.Pinned
	if n.PinnedOnly && len(n.Pinned) == 0 {
		n.PinnedOnly = false // nothing to use: would only fail to connect
	}
	if err := s.x.Settings.Save(n); err != nil {
		return err
	}
	if s.x.OnSettingsChanged != nil {
		s.x.OnSettingsChanged(old, n)
	}
	if dpiChanged(old.DPI, n.DPI) {
		if err := s.o.RestartDPI(context.Background()); err != nil {
			return err
		}
	}
	if proxyPhaseChanged(old.Proxy, n.Proxy) {
		// May wait for the SYSPROXY_EXISTING answer: do not block the UI call.
		s.background(func(ctx context.Context) { _ = s.o.ReapplyProxy(ctx) })
	}
	if dnsServerChanged(old.DNSServer, n.DNSServer) {
		s.background(func(ctx context.Context) { _ = s.o.ReapplyDNSServer(ctx) })
	}
	if old.FakeSNI != n.FakeSNI {
		s.background(func(ctx context.Context) { _ = s.o.ReapplyFakeSNI(ctx) })
	}
	return nil
}

// SetMode switches between the simple and the full interface and resizes
// the window.
func (s *Service) SetMode(mode string) error {
	if mode != store.ModeSimple && mode != store.ModeFull {
		return fmt.Errorf("mode must be simple or full")
	}
	st := s.x.Settings.Get()
	st.Mode = mode
	if err := s.x.Settings.Save(st); err != nil {
		return err
	}
	if s.x.SetMode != nil {
		s.x.SetMode(mode)
	}
	return nil
}

// ListServers returns every server with its last result.
func (s *Service) ListServers() []ServerRow {
	results := map[string]scanner.Result{}
	for _, r := range s.o.ScanResults() {
		results[r.ServerID] = r
	}
	st := s.x.Settings.Get()
	s.o.mu.Lock()
	inUse := map[string]bool{}
	for _, sv := range s.o.servers {
		inUse[sv.ID] = true
	}
	s.o.mu.Unlock()
	var rows []ServerRow
	for _, sv := range s.x.Catalog() {
		row := ServerRow{Server: sv, InUse: inUse[sv.ID], Pinned: slices.Contains(st.Pinned, sv.ID), Auto: Eligible(st, sv)}
		if r, ok := results[sv.ID]; ok {
			row.Result = &r
		}
		rows = append(rows, row)
	}
	return rows
}

// ScanAll rebuilds the server ranking in the background (progress arrives as
// events) and, when connected, switches to its best servers.
func (s *Service) ScanAll() error {
	s.mu.Lock()
	if s.scanCancel != nil {
		s.mu.Unlock()
		return errors.New("scan already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanCancel = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.scanCancel = nil
			s.mu.Unlock()
			cancel()
		}()
		// A picker that reports its own scans does so for this one too.
		w, ok := s.o.d.Scans.(interface{ Watched() bool })
		watched := ok && w.Watched()
		var onProgress func(done, total int, r scanner.Result)
		if !watched {
			onProgress = func(done, total int, r scanner.Result) {
				s.x.Bus.Emit(EventScan, ScanProgress{Done: done, Total: total, Result: &r, Running: true})
			}
		}
		_, err := s.o.Rescan(ctx, onProgress)
		if err == nil {
			s.o.ApplyBest(ctx)
		}
		if !watched {
			s.x.Bus.Emit(EventScan, ScanProgress{Running: false})
		}
	}()
	return nil
}

// CancelScan stops a running scan.
func (s *Service) CancelScan() {
	s.mu.Lock()
	c := s.scanCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// AddServers imports URLs/stamps (one per line). It returns how many were
// added and the lines that were rejected.
func (s *Service) AddServers(text string) (int, []string) {
	add, bad := servers.ParseImport([]byte(text))
	cur, err := s.x.LoadCustom()
	if err != nil {
		return 0, append(bad, err.Error())
	}
	seen := map[string]bool{}
	for _, c := range cur {
		seen[c.Address] = true
	}
	n := 0
	for _, a := range add {
		if seen[a.Address] {
			continue
		}
		seen[a.Address] = true
		cur = append(cur, a)
		n++
	}
	if err := s.x.SaveCustom(cur); err != nil {
		return 0, append(bad, err.Error())
	}
	if bad == nil {
		bad = []string{}
	}
	return n, bad
}

// RemoveCustomServer deletes a user-added server.
func (s *Service) RemoveCustomServer(id string) error {
	cur, err := s.x.LoadCustom()
	if err != nil {
		return err
	}
	cur = slices.DeleteFunc(cur, func(m model.Server) bool { return m.ID == id })
	return s.x.SaveCustom(cur)
}

// SetPinned pins or unpins a server.
func (s *Service) SetPinned(id string, pinned bool) error {
	st := s.x.Settings.Get()
	st.Pinned = slices.DeleteFunc(slices.Clone(st.Pinned), func(p string) bool { return p == id })
	if pinned {
		st.Pinned = append(st.Pinned, id)
	}
	return s.x.Settings.Save(st)
}

// SetPinnedMany pins or unpins several servers with one save (bulk pin
// from search results, "unpin all"). Order is kept and duplicates dropped.
func (s *Service) SetPinnedMany(ids []string, pinned bool) error {
	st := s.x.Settings.Get()
	cur := slices.Clone(st.Pinned)
	if pinned {
		for _, id := range ids {
			if !slices.Contains(cur, id) {
				cur = append(cur, id)
			}
		}
	} else {
		cur = slices.DeleteFunc(cur, func(p string) bool { return slices.Contains(ids, p) })
	}
	if cur == nil {
		cur = []string{}
	}
	st.Pinned = cur
	if len(cur) == 0 {
		st.PinnedOnly = false
	}
	return s.x.Settings.Save(st)
}

// UseOnlyServer makes id the only pinned server and turns "use pinned
// servers only" on, in one save.
func (s *Service) UseOnlyServer(id string) error {
	if id == "" {
		return errors.New("app: empty server id")
	}
	st := s.x.Settings.Get()
	st.Pinned, st.PinnedOnly = []string{id}, true
	return s.x.Settings.Save(st)
}

// CheckServer re-tests one server and returns its updated row.
func (s *Service) CheckServer(id string) (ServerRow, error) {
	if s.x.CheckServer == nil {
		return ServerRow{}, errors.New("app: server check unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.x.CheckServer(ctx, id); err != nil {
		return ServerRow{}, err
	}
	for _, r := range s.ListServers() {
		if r.Server.ID == id {
			return r, nil
		}
	}
	return ServerRow{}, fmt.Errorf("app: no server %q", id)
}

// SetDPIEnabled turns GoodbyeDPI on or off.
func (s *Service) SetDPIEnabled(on bool) error { return s.o.SetDPIEnabled(context.Background(), on) }

// StartAutotune runs DPI autotune in the background with progress events.
func (s *Service) StartAutotune() error {
	s.mu.Lock()
	if s.tuneCancel != nil {
		s.mu.Unlock()
		return errors.New("autotune already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.tuneCancel = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.tuneCancel = nil
			s.mu.Unlock()
			cancel()
		}()
		err := s.o.Autotune(ctx, func(engine, p string, i, n int) {
			s.x.Bus.Emit(EventAutotune, AutotuneProgress{Engine: engine, Preset: p, Index: i, Total: n, Running: true})
		})
		final := AutotuneProgress{Running: false}
		if err == nil { // name what was picked, for the "done" message
			d := s.o.Snapshot().DPI
			final.Engine, final.Preset = d.Engine, d.Preset
		}
		var ae *AppError
		if errors.As(err, &ae) {
			final.Error = &AppError{Code: ae.Code, Params: ae.Params}
		}
		s.x.Bus.Emit(EventAutotune, final)
	}()
	return nil
}

// CancelAutotune stops autotune.
func (s *Service) CancelAutotune() {
	s.mu.Lock()
	c := s.tuneCancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

// ProbeNow probes the configured sites once.
func (s *Service) ProbeNow() []probe.Result { return s.o.ProbeNow(context.Background()) }

// GetDPIBlacklist reads the blacklist file ("" when missing).
func (s *Service) GetDPIBlacklist() (string, error) {
	b, err := os.ReadFile(s.x.Paths.DPIBlacklist)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

// SaveDPIBlacklist writes the blacklist file and applies it now: zapret2
// re-reads it by itself, GoodbyeDPI is restarted. An empty list is refused:
// the engine would then bypass nothing.
func (s *Service) SaveDPIBlacklist(text string) error {
	if dpi.BlacklistEntries(text) == 0 {
		return appErr(CodeDPIBlacklistEmpty, nil)
	}
	if err := os.MkdirAll(s.x.Paths.DataDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.x.Paths.DPIBlacklist, []byte(text), 0o644); err != nil {
		return err
	}
	return s.o.RefreshDPILists(context.Background())
}

// validateDPI checks the engine and each engine's custom args.
func (s *Service) validateDPI(d store.DPISettings) error {
	if d.Engine != store.EngineGoodbyeDPI && d.Engine != store.EngineZapret2 {
		return fmt.Errorf("settings: unknown DPI engine %q", d.Engine)
	}
	check := func(engine, custom string, used bool) error {
		if !used && custom == "" {
			return nil
		}
		e, ok := s.o.d.DPI.Get(engine)
		if !ok {
			return nil
		}
		if err := e.ValidateCustom(custom); err != nil {
			return appErr(CodeDPICustomRejected, err, "engine", engine, "arg", err.Error())
		}
		return nil
	}
	if err := check(store.EngineGoodbyeDPI, d.CustomArgs, d.Preset == "custom"); err != nil {
		return err
	}
	return check(store.EngineZapret2, d.Zapret2.CustomArgs, d.Zapret2.Strategy == "custom")
}

// dpiChanged reports a change that a running engine must restart for.
func dpiChanged(a, b store.DPISettings) bool {
	return a.Engine != b.Engine || a.Preset != b.Preset || a.CustomArgs != b.CustomArgs || a.Scope != b.Scope ||
		a.Zapret2 != b.Zapret2
}

// DPIStrategies lists an engine's strategies in autotune order.
func (s *Service) DPIStrategies(engine string) ([]dpi.Strategy, error) {
	e, ok := s.o.d.DPI.Get(engine)
	if !ok {
		return nil, fmt.Errorf("%w: %q", dpi.ErrUnknownEngine, engine)
	}
	return e.Strategies(), nil
}

// PreviewDPIArgs shows the command line an engine would get for the given
// options (list paths as stored in the data directory).
func (s *Service) PreviewDPIArgs(engine, strategy, custom, scope string, autoHostlist bool) ([]string, error) {
	e, ok := s.o.d.DPI.Get(engine)
	if !ok {
		return nil, fmt.Errorf("%w: %q", dpi.ErrUnknownEngine, engine)
	}
	p := dpi.Plan{Strategy: strategy, Custom: custom, Scope: dpi.Scope(scope), Blacklist: s.x.Paths.DPIBlacklist}
	if autoHostlist {
		p.AutoHostlist = s.x.Paths.DPIAutoHostlist
	}
	return e.Args(p)
}

// GetDPIAutoHostlist returns the sites zapret2 detected as blocked, sorted.
func (s *Service) GetDPIAutoHostlist() ([]string, error) {
	b, err := os.ReadFile(s.o.d.AutoHostlistPath)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	return cleanDomains(strings.Split(string(b), "\n")), nil
}

// SaveDPIAutoHostlist replaces the auto-detected sites (the user removed
// some) and restarts a running zapret2: it keeps its own copy and would
// write the removed sites back.
func (s *Service) SaveDPIAutoHostlist(domains []string) error {
	text := strings.Join(cleanDomains(domains), "\n")
	if text != "" {
		text += "\n"
	}
	return s.o.RewriteZapret2File(context.Background(), func() error {
		if err := os.MkdirAll(filepath.Dir(s.o.d.AutoHostlistPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(s.o.d.AutoHostlistPath, []byte(text), 0o644)
	})
}

// DPIEngineDir is the folder an engine runs from, for antivirus exclusions.
func (s *Service) DPIEngineDir(engine string) string { return filepath.Join(s.x.Paths.BinDir, engine) }

// RetryZapret2 restarts DPI with the configured engine after a fallback.
func (s *Service) RetryZapret2() error { return s.o.RestartDPI(context.Background()) }

// cleanDomains trims, lower-cases, drops blanks and comments, dedupes and
// sorts.
func cleanDomains(in []string) []string {
	out := []string{}
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" && !strings.HasPrefix(d, "#") && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	slices.Sort(out)
	return out
}

// DismissWarning hides a warning. RESTORE_FAILED cannot be dismissed: only
// a successful restore clears it.
func (s *Service) DismissWarning(code string) {
	if code == CodeRestoreFailed {
		return
	}
	s.o.ClearWarning(code)
}

// RestoreDNSNow forces a DNS restore.
func (s *Service) RestoreDNSNow() error {
	return s.o.RestoreNow(context.Background(), s.x.RestoreNow)
}

// StopConflictingService stops the service holding port 53. The UI calls
// it only after the user confirmed in-page; the name must be one of the
// port's current owners, so the call cannot stop anything else.
func (s *Service) StopConflictingService(name string) error {
	owners, err := s.o.d.System.PortOwners(53)
	if err != nil {
		return err
	}
	for _, o := range owners {
		if name != "" && o.Service == name {
			return s.x.StopService(name)
		}
	}
	return appErr(CodePort53NotOwner, nil, "name", name)
}

// DNSInfo describes how Ghostline changes this system's DNS (Settings
// shows the adapter choice, or the backend chain).
func (s *Service) DNSInfo() sysdns.Info {
	if s.x.DNSInfo == nil {
		return sysdns.Info{Interfaces: []string{}}
	}
	return s.x.DNSInfo()
}

// ListAdapters lists network adapters for manual selection.
func (s *Service) ListAdapters() []sysdns.Adapter {
	ads, _ := s.x.ListAdapters()
	return ads
}

// LANDNSClients counts LAN devices that used the DNS server in the last 10
// minutes (the tray asks before cutting them off).
func (s *Service) LANDNSClients() int {
	if s.x.LANDNSClients == nil {
		return 0
	}
	return s.x.LANDNSClients()
}

// CheckUpdateNow checks for a newer release right away, ignoring the
// schedule and the "notify about new versions" setting.
func (s *Service) CheckUpdateNow() (UpdateCheck, error) {
	if s.x.CheckUpdate == nil {
		return UpdateCheck{}, errors.New(CodeUpdateCheckFailed)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := s.x.CheckUpdate(ctx)
	if err != nil {
		return UpdateCheck{}, fmt.Errorf("%s: %w", CodeUpdateCheckFailed, err)
	}
	return r, nil
}

// AppInfo returns version and update information.
func (s *Service) AppInfo() AppInfo { return s.x.Info() }

// SetQueryLog toggles the RAM-only live query view.
func (s *Service) SetQueryLog(on bool) {
	s.x.Bus.queryLog.Store(on)
	if !on {
		s.x.Bus.queries.Reset()
		s.x.Bus.conns.Reset()
	}
}

// GetQueries returns the RAM-only query view.
func (s *Service) GetQueries() []engine.QueryEvent { return s.x.Bus.queries.All() }

// GetLogs returns the in-memory log.
func (s *Service) GetLogs() []LogEvent { return s.x.Bus.logs.All() }

// RunStats emits StatsEvent on every tick while connected. It is a
// function, not a method, so Wails does not bind it.
func RunStats(s *Service, ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
		if !s.o.connected() {
			continue
		}
		st := s.o.d.Engine.Stats()
		ev := StatsEvent{Queries: st.Queries, LatencyMs: int(st.AvgLatency / time.Millisecond)}
		s.o.update(func(sn *Snapshot) { sn.Queries, sn.LatencyMs = ev.Queries, ev.LatencyMs })
		s.x.Bus.Emit(EventStats, ev)
	}
}
