package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Settings is the user configuration (spec §9).
type Settings struct {
	Version          int               `json:"version"`
	Language         string            `json:"language"`
	Mode             string            `json:"mode"`
	StartWithWindows bool              `json:"startWithWindows"`
	AutoConnect      bool              `json:"autoConnect"`
	CloseToTray      bool              `json:"closeToTray"`
	Adapters         string            `json:"adapters"` // "auto" | "manual"
	AdapterGUIDs     []string          `json:"adapterGuids,omitempty"`
	TestDomain       string            `json:"testDomain"`
	Bootstrap        []string          `json:"bootstrap"`
	MaxUpstreams     int               `json:"maxUpstreams"`
	IncludeTags      []string          `json:"includeTags"`
	Pinned           []string          `json:"pinned"`
	PinnedOnly       bool              `json:"pinnedOnly"`
	ProbeSites       []string          `json:"probeSites"`
	DPI              DPISettings       `json:"dpi"`
	FragmentDNS      FragmentSettings  `json:"fragmentDns"`
	Updates          UpdateSettings    `json:"updates"`
	FullWindow       WindowSize        `json:"fullWindow"`
	Proxy            ProxySettings     `json:"proxy"`
	DNSBlockMode     string            `json:"dnsBlockMode"` // "zero" | "nxdomain"
	DNSServer        DNSServerSettings `json:"dnsServer"`
	FakeSNI          FakeSNISettings   `json:"fakeSni"`
	Tools            ToolsSettings     `json:"tools"`
	Simple           SimpleSettings    `json:"simple"`
}

// SimpleSettings belong to the Simple interface's protection levels.
type SimpleSettings struct {
	// Custom is the user's own combination, remembered when they switch to
	// a level so "custom" can bring it back. Nil until there is one.
	Custom *SimpleCustom `json:"custom,omitempty"`
	// Checked is set once the first-run tune ran: on the first connect the
	// Simple interface checks the test sites and auto-tunes DPI if needed.
	Checked bool `json:"checked"`
}

// SimpleCustom is a combination of the switches the protection levels set.
type SimpleCustom struct {
	DPI         bool `json:"dpi"`
	Proxy       bool `json:"proxy"`
	SystemProxy bool `json:"systemProxy"`
	FakeSNI     bool `json:"fakeSni"`
}

// ToolsSettings configure the Tools page (phase 3).
type ToolsSettings struct {
	Scanner ScannerTool `json:"scanner"`
	CFScan  CFScanTool  `json:"cfscan"`
}

// ScannerTool configures the advanced DNS scanner.
type ScannerTool struct {
	Rounds     int `json:"rounds"`
	Workers    int `json:"workers"`
	TimeoutMs  int `json:"timeoutMs"`
	MaxServers int `json:"maxServers"` // servers graded per scan
}

// Advanced scan size: default and allowed range.
const (
	DefaultScanMaxServers = 500
	MinScanMaxServers     = 50
	MaxScanMaxServers     = 2000
)

// CFScanTool configures the Cloudflare clean-IP scan.
type CFScanTool struct {
	Host        string `json:"host"`
	MaxIPs      int    `json:"maxIps"`
	Want        int    `json:"want"`
	Concurrency int    `json:"concurrency"`
	TimeoutMs   int    `json:"timeoutMs"`
	SpeedTest   bool   `json:"speedTest"`
	SpeedBytes  int    `json:"speedBytes"`
}

// DefaultTools returns the spec 3 §11 defaults.
func DefaultTools() ToolsSettings {
	return ToolsSettings{
		Scanner: ScannerTool{Rounds: 5, Workers: 8, TimeoutMs: 3000, MaxServers: DefaultScanMaxServers},
		CFScan: CFScanTool{Host: "speed.cloudflare.com", MaxIPs: 2000, Want: 50, Concurrency: 64,
			TimeoutMs: 2000, SpeedTest: true, SpeedBytes: 1048576},
	}
}

var hostLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidHostname reports whether h is a DNS name (not an IP), at most 253
// characters, with labels of letters, digits and inner hyphens.
func ValidHostname(h string) bool {
	if h == "" || len(h) > 253 || net.ParseIP(h) != nil {
		return false
	}
	for _, l := range strings.Split(h, ".") {
		if !hostLabel.MatchString(l) {
			return false
		}
	}
	return true
}

// ValidateTools checks tool options against the spec 3 §6.1, §7 and §11 ranges.
func ValidateTools(t ToolsSettings) error {
	sc, cf := t.Scanner, t.CFScan
	switch {
	case sc.Rounds < 3 || sc.Rounds > 20:
		return fmt.Errorf("tools: scanner rounds must be 3..20")
	case sc.Workers < 4 || sc.Workers > 32:
		return fmt.Errorf("tools: scanner workers must be 4..32")
	case sc.TimeoutMs < 1000 || sc.TimeoutMs > 10000:
		return fmt.Errorf("tools: scanner timeoutMs must be 1000..10000")
	case sc.MaxServers < MinScanMaxServers || sc.MaxServers > MaxScanMaxServers:
		return fmt.Errorf("tools: scanner maxServers must be %d..%d", MinScanMaxServers, MaxScanMaxServers)
	case !ValidHostname(cf.Host):
		return fmt.Errorf("tools: cfscan host must be a domain name")
	case cf.MaxIPs < 200 || cf.MaxIPs > 10000:
		return fmt.Errorf("tools: cfscan maxIps must be 200..10000")
	case cf.Want < 0 || cf.Want > 1000:
		return fmt.Errorf("tools: cfscan want must be 0..1000")
	case cf.Concurrency < 8 || cf.Concurrency > 128:
		return fmt.Errorf("tools: cfscan concurrency must be 8..128")
	case cf.TimeoutMs < 1000 || cf.TimeoutMs > 5000:
		return fmt.Errorf("tools: cfscan timeoutMs must be 1000..5000")
	case cf.SpeedBytes < 102400 || cf.SpeedBytes > 26214400:
		return fmt.Errorf("tools: cfscan speedBytes must be 102400..26214400")
	}
	return nil
}

// ProxySettings configures the local proxy (phase 2A).
type ProxySettings struct {
	Enabled     bool            `json:"enabled"`
	Port        int             `json:"port"`
	SystemProxy bool            `json:"systemProxy"`
	ShareLAN    bool            `json:"shareLan"`
	Fragment    WebFragment     `json:"fragment"`
	Upstreams   []UpstreamProxy `json:"upstreams"`
}

// WebFragment configures ClientHello fragmentation for proxied traffic.
type WebFragment struct {
	Mode          string `json:"mode"`   // auto | always | never
	Method        string `json:"method"` // tcp | record | both
	Chunks        int    `json:"chunks"`
	DelayMs       int    `json:"delayMs"`
	AutoTimeoutMs int    `json:"autoTimeoutMs"`
	CacheDays     int    `json:"cacheDays"`
}

// UpstreamProxy is a proxy rules can send traffic through. PassEnc is the
// DPAPI-protected password, base64.
type UpstreamProxy struct {
	ID      string `json:"id"`
	Type    string `json:"type"` // socks5 | http
	Addr    string `json:"addr"` // host:port
	User    string `json:"user"`
	PassEnc string `json:"passEnc"`
}

var upstreamID = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateProxy checks proxy settings against the spec section 9 ranges.
func ValidateProxy(p ProxySettings) error {
	f := p.Fragment
	switch {
	case p.Port < 1024 || p.Port > 65535:
		return fmt.Errorf("proxy: port must be 1024..65535")
	case f.Mode != "auto" && f.Mode != "always" && f.Mode != "never":
		return fmt.Errorf("proxy: fragment mode must be auto, always or never")
	case f.Method != "tcp" && f.Method != "record" && f.Method != "both":
		return fmt.Errorf("proxy: fragment method must be tcp, record or both")
	case f.Chunks < 2 || f.Chunks > 64:
		return fmt.Errorf("proxy: chunks must be 2..64")
	case f.DelayMs < 0 || f.DelayMs > 100:
		return fmt.Errorf("proxy: delayMs must be 0..100")
	case f.AutoTimeoutMs < 1000 || f.AutoTimeoutMs > 10000:
		return fmt.Errorf("proxy: autoTimeoutMs must be 1000..10000")
	case f.CacheDays < 1 || f.CacheDays > 90:
		return fmt.Errorf("proxy: cacheDays must be 1..90")
	}
	seen := map[string]bool{}
	for _, u := range p.Upstreams {
		if !upstreamID.MatchString(u.ID) || seen[u.ID] {
			return fmt.Errorf("proxy: upstream id %q must be unique lower-case letters, digits or '-'", u.ID)
		}
		seen[u.ID] = true
		if u.Type != "socks5" && u.Type != "http" {
			return fmt.Errorf("proxy: upstream %q type must be socks5 or http", u.ID)
		}
		host, port, err := net.SplitHostPort(u.Addr)
		if n, perr := strconv.Atoi(port); err != nil || perr != nil || host == "" || n < 1 || n > 65535 {
			return fmt.Errorf("proxy: upstream %q address must be host:port", u.ID)
		}
	}
	return nil
}

// DPISettings configures the DPI bypass engine. Preset and CustomArgs are
// GoodbyeDPI's (kept at this level for older files); zapret2 has its own.
type DPISettings struct {
	Enabled        bool            `json:"enabled"`
	Engine         string          `json:"engine"` // "goodbyedpi" | "zapret2"
	Preset         string          `json:"preset"`
	CustomArgs     string          `json:"customArgs"`
	Scope          string          `json:"scope"`
	Zapret2        Zapret2Settings `json:"zapret2"`
	HideEngineHint bool            `json:"hideEngineHint"`
}

// Zapret2Settings is the zapret2 engine's part of DPISettings.
type Zapret2Settings struct {
	Strategy     string `json:"strategy"`
	CustomArgs   string `json:"customArgs"`
	AutoHostlist bool   `json:"autoHostlist"`
}

// DPI engine IDs.
const (
	EngineGoodbyeDPI = "goodbyedpi"
	EngineZapret2    = "zapret2"
)

type FragmentSettings struct {
	Enabled bool `json:"enabled"`
	Chunks  int  `json:"chunks"`
	DelayMs int  `json:"delayMs"`
}

type UpdateSettings struct {
	CheckApp         bool `json:"checkApp"`
	UpdateServerList bool `json:"updateServerList"`
	// Beta also announces pre-releases (tags like v0.8.0-beta.1).
	Beta bool `json:"beta"`
}

type WindowSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Interface modes (Settings.Mode). Files before v0.5 said "advanced" for
// the full interface; MigrateSettings renames it.
const (
	ModeSimple = "simple"
	ModeFull   = "full"
)

// DefaultSettings returns the spec §9 defaults.
func DefaultSettings() Settings {
	return Settings{
		Version:      5,
		Language:     "vi",
		Mode:         ModeSimple,
		CloseToTray:  true,
		Adapters:     "auto",
		TestDomain:   "www.google.com",
		Bootstrap:    []string{"1.1.1.1:53", "8.8.8.8:53"},
		MaxUpstreams: 5,
		IncludeTags:  []string{"no-filter"},
		Pinned:       []string{},
		ProbeSites:   []string{"youtube.com", "discord.com", "x.com"},
		DPI:          DPISettings{Engine: EngineZapret2, Preset: "light", Scope: "all", Zapret2: Zapret2Settings{Strategy: "z-split"}},
		FragmentDNS:  FragmentSettings{Chunks: 5, DelayMs: 5},
		Updates:      UpdateSettings{CheckApp: true, UpdateServerList: true},
		FullWindow:   WindowSize{Width: 1000, Height: 660},
		Proxy: ProxySettings{
			Port:      8080,
			Fragment:  WebFragment{Mode: "auto", Method: "both", Chunks: 5, DelayMs: 5, AutoTimeoutMs: 3000, CacheDays: 7},
			Upstreams: []UpstreamProxy{},
		},
		DNSBlockMode: "zero",
		DNSServer:    DNSServerSettings{DoHPort: 443},
		Tools:        DefaultTools(),
	}
}

// DNSServerSettings configure the DNS server for this PC and the LAN.
type DNSServerSettings struct {
	Enabled  bool   `json:"enabled"`  // DoH on loopback
	ShareLAN bool   `json:"shareLan"` // DoH and port 53 on LAN addresses too
	DoHPort  int    `json:"dohPort"`
	IOSSSID  string `json:"iosSsid"` // home Wi-Fi for the iOS profile
}

// FakeSNISettings configure Fake SNI. AckVersion is the warning version the
// user confirmed; Fake SNI runs only when it is current.
type FakeSNISettings struct {
	Enabled    bool `json:"enabled"`
	AckVersion int  `json:"ackVersion"`
}

// SetupPagePort is the phone setup page's port (spec 2B 7.2).
const SetupPagePort = 8053

// ValidateDNSServer checks DNS server settings (spec 2B 10).
func ValidateDNSServer(d DNSServerSettings, proxyPort int) error {
	switch {
	case d.DoHPort < 1 || d.DoHPort > 65535:
		return fmt.Errorf("dnsServer: dohPort must be 1..65535")
	case d.DoHPort == 53 || d.DoHPort == SetupPagePort || d.DoHPort == proxyPort:
		return fmt.Errorf("dnsServer: dohPort must differ from 53, %d and the proxy port", SetupPagePort)
	case len(d.IOSSSID) > 32:
		return fmt.Errorf("dnsServer: the Wi-Fi name is longer than 32 bytes")
	}
	return nil
}

// LoadSettings reads settings, filling missing fields with defaults. A file
// that cannot be parsed is renamed to settings.json.bak and defaults are
// returned with recovered=true.
func LoadSettings(path string) (s Settings, recovered bool, err error) {
	s = DefaultSettings()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	m, jerr := MigrateSettings(b)
	if jerr != nil {
		slog.Warn("store: settings.json is not valid JSON; reset to defaults, old file kept as .bak", "path", path, "err", jerr)
		if err := os.Rename(path, path+".bak"); err != nil {
			return DefaultSettings(), true, err
		}
		return DefaultSettings(), true, nil
	}
	return m, false, nil
}

// MigrateSettings parses a settings file of any version (v1–v5) into the
// current version, filling missing fields with defaults. It fails only on
// invalid JSON.
func MigrateSettings(b []byte) (Settings, error) {
	s := DefaultSettings()
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // tolerate a UTF-8 BOM (Notepad)
	if err := json.Unmarshal(b, &s); err != nil {
		return DefaultSettings(), err
	}
	// v1 files gain the v2 defaults through the pre-filled struct. Files
	// older than v3 predate zapret2: their users keep GoodbyeDPI.
	var head struct {
		Version        int
		FullWindow     *WindowSize `json:"fullWindow"`
		AdvancedWindow *WindowSize `json:"advancedWindow"`
	}
	_ = json.Unmarshal(b, &head)
	if s.Mode == "advanced" {
		s.Mode = ModeFull
	}
	if head.FullWindow == nil && head.AdvancedWindow != nil {
		s.FullWindow = *head.AdvancedWindow
	}
	if head.Version < 3 {
		s.DPI.Engine = EngineGoodbyeDPI
	}
	if s.DPI.Engine != EngineGoodbyeDPI && s.DPI.Engine != EngineZapret2 {
		s.DPI.Engine = EngineZapret2
	}
	s.Version = 5
	if s.DNSServer.DoHPort == 0 {
		s.DNSServer.DoHPort = 443
	}
	if s.Proxy.Upstreams == nil {
		s.Proxy.Upstreams = []UpstreamProxy{}
	}
	if slices.Equal(s.ProbeSites, oldProbeSites) {
		s.ProbeSites = DefaultSettings().ProbeSites
	}
	if head.Version < 5 {
		s.Tools = DefaultTools()
		s.Simple.Checked = true // upgrading from v0.4 or older: not a new user
	}
	if s.Tools.Scanner.MaxServers == 0 { // v5 files from before maxServers
		s.Tools.Scanner.MaxServers = DefaultScanMaxServers
	}
	return s, nil
}

// oldProbeSites is the default test-site list before v0.2.5. Files that
// still hold it untouched move to the current default; edited lists stay.
var oldProbeSites = []string{"youtube.com", "discord.com", "telegram.org", "x.com"}

// SaveSettings writes settings atomically.
func SaveSettings(path string, s Settings) error {
	return WriteJSONAtomic(path, s)
}
