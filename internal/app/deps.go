package app

import (
	"context"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"net/netip"
	"time"

	"github.com/AdguardTeam/dnsproxy/upstream"
	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/probe"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/watchdog"
)

// Engine is the loopback DNS server.
type Engine interface {
	Start(context.Context, engine.Config) error
	Swap(context.Context, []upstream.Upstream) error
	Stop(context.Context) error
	SelfTest(context.Context) error
	ExpectVerify(string)
	SawVerify(string) bool
	Stats() engine.Stats
}

// DNS changes the system's DNS (sysdns.Backend: adapters on Windows,
// NetworkManager, systemd-resolved or resolv.conf on Linux).
type DNS = sysdns.Backend

// DPI runs one DPI bypass engine at a time (dpi.Manager).
type DPI interface {
	Start(ctx context.Context, engine string, p dpi.Plan) (int, error)
	Stop() error
	Running() bool
	Engine() string // engine running, "" when stopped
	RefreshLists(p dpi.Plan) error
	Get(engine string) (dpi.Engine, bool)
}

// Safety starts the watchdog and the logon recovery task.
type Safety interface {
	StartWatchdog(pid uint32, start time.Time) (stop func() error, err error)
	CreateRecoveryTask() error
	DeleteRecoveryTask() error
}

// System answers questions about the machine.
type System interface {
	IsAdmin() bool
	PortOwners(uint16) ([]procs.PortOwner, error)
	// ListenFree binds UDP and TCP on each address, then releases them.
	ListenFree([]netip.AddrPort) error
	SelfPID() (uint32, time.Time)
	IPv6Available() bool
}

// Picker chooses the upstream servers to use.
type Picker interface {
	Pick(ctx context.Context, onProgress func(done, total int)) ([]model.Server, error)
}

// Builder turns servers into upstreams.
type Builder interface {
	Build(model.Server) (upstream.Upstream, error)
}

// Resolver is the system resolver, used only for leak verification.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Prober probes sites.
type Prober interface {
	ProbeAll(ctx context.Context, sites []string) []probe.Result
}

// States persists the write-ahead connection state.
type States interface {
	Load() (store.State, error)
	Update(func(*store.State) error) error
}

// Sink receives UI events.
type Sink interface {
	State(Snapshot)
	Log(LogEvent)
}

// ProxyRun describes one start of the local proxy.
type ProxyRun struct {
	Listen   []netip.AddrPort
	ShareLAN bool
}

// Proxy is the local HTTP/SOCKS proxy.
type Proxy interface {
	Start(ctx context.Context, run ProxyRun) error
	Stop(ctx context.Context) error
	SelfTest(ctx context.Context) error
	Alive() bool
}

// SysProxy changes the system proxy (WinINET, or the desktop's settings).
type SysProxy = sysproxy.Backend

// Firewall manages Ghostline's inbound rules.
type Firewall interface {
	Add(port int) error // the proxy's LAN-sharing rule
	Delete() error
	AddNamed(r firewall.Rule) error // DNS server and setup page rules
	DeleteNamed(name string) error
}

// DNSServer runs the DoH and LAN DNS listeners (engine.Serve).
type DNSServer interface {
	Serve(ctx context.Context, sc engine.ServeConfig) (engine.ServeResult, error)
	StopServe(ctx context.Context) error
	// SelfTest queries the engine through DoH on loopback.
	SelfTest(ctx context.Context) error
}

// Certs manages Ghostline's root certificates in the system store.
type Certs interface {
	// LANCA loads or creates the LAN CA and makes sure it is installed.
	LANCA(ctx context.Context) (*certs.CA, error)
	ResetLANCA(ctx context.Context) (*certs.CA, error)
	RemoveLANCA(ctx context.Context) error
	InstallSession(der []byte) error
	RemoveSession(thumbprint string) error
	List() ([]certstore.Cert, error)
}

// Deps wires the orchestrator.
type Deps struct {
	Engine       Engine
	DNS          DNS
	DPI          DPI
	Safety       Safety
	System       System
	Picker       Picker
	Builder      Builder
	Resolver     Resolver
	Prober       Prober
	Scans        Scans
	Recover      func() (watchdog.Outcome, error)
	Sink         Sink
	States       States
	Settings     func() store.Settings
	SaveSettings func(store.Settings) error
	Now          func() time.Time
	Sleep        func(time.Duration)
	// Ticker returns a tick channel and a stop func (health checks).
	Ticker        func(time.Duration) (<-chan time.Time, func())
	BlacklistPath string
	// AutoHostlistPath is where zapret2's auto-detected sites are kept.
	AutoHostlistPath string
	// Proxy phase (phase 2A). A nil Proxy disables the phase.
	Proxy    Proxy
	SysProxy SysProxy
	Firewall Firewall
	// ConfirmOverride asks the user before replacing another app's system
	// proxy or PAC (SYSPROXY_EXISTING); nil means "do not replace". ctx is
	// cancelled by Disconnect, which must not wait for the answer.
	ConfirmOverride func(ctx context.Context, server, pac string) bool
	// Rules returns the current rules for the DNS engine; nil means none.
	Rules    func() *rules.Compiled
	ListenV4 netip.AddrPort // default 127.0.0.1:53
	ListenV6 netip.AddrPort // default [::1]:53
	// DNS server and Fake SNI (phase 2B). A nil DNSServer or Certs
	// disables the phases that need them.
	DNSServer DNSServer
	Certs     Certs
	LANAddrs  func() []netip.Addr
	// SetMITM hands the Fake SNI certificate source to the proxy; nil
	// turns Fake SNI off. MITMSelfTest intercepts a loopback test server.
	SetMITM      func(mitm.LeafSource)
	MITMSelfTest func(ctx context.Context) error
	// AfterFunc is time.AfterFunc (tests replace it); the result stops
	// the timer.
	AfterFunc func(d time.Duration, f func()) (stop func() bool)
}
