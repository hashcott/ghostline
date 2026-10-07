package app

import (
	"log/slog"
	"sync/atomic"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/hashcott/ghostline/internal/scanner"
)

// UI event names.
const (
	EventState    = "state"
	EventStats    = "stats"
	EventLog      = "log"
	EventQuery    = "query"
	EventScan     = "scan:progress"
	EventAutotune = "dpi:autotune"
	EventUpdate   = "update"

	EventProxyStats    = "proxy:stats"
	EventProxyConn     = "proxy:conn"
	EventRulesCompiled = "rules:compiled"
	EventListsProgress = "lists:progress"

	EventDNSServerStats = "dnsserver:stats"
	EventCertsChanged   = "certs:changed"
	EventSetupCountdown = "setup:countdown"

	// Phase 3 tools.
	EventToolsScan   = "tools:scan"
	EventToolsCFScan = "tools:cfscan"
)

// SetupCountdown is the phone setup page's remaining time (0: closed).
type SetupCountdown struct {
	URL          string `json:"url"`
	RemainingSec int    `json:"remainingSec"`
}

// StatsEvent is emitted every second while protected.
type StatsEvent struct {
	Queries   uint64 `json:"queries"`
	LatencyMs int    `json:"latencyMs"`
}

// ScanProgress reports a full scan.
type ScanProgress struct {
	Done    int             `json:"done"`
	Total   int             `json:"total"`
	Result  *scanner.Result `json:"result,omitempty"`
	Running bool            `json:"running"`
}

// AutotuneProgress reports DPI autotune.
type AutotuneProgress struct {
	Engine  string    `json:"engine"`
	Preset  string    `json:"preset"`
	Index   int       `json:"index"`
	Total   int       `json:"total"`
	Running bool      `json:"running"`
	Error   *AppError `json:"error,omitempty"`
}

// UpdateCheck is the result of a manual "check for updates".
type UpdateCheck struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	URL     string `json:"url"`
	Newer   bool   `json:"newer"`
}

// UpdateInfo announces a newer release.
type UpdateInfo struct {
	Tag string `json:"tag"`
	URL string `json:"url"`
}

// Emitter sends events to the UI.
type Emitter interface {
	Emit(name string, data any)
}

// Bus implements Sink: it forwards state and logs to the UI and keeps the
// log and (when enabled) the query view in RAM. Nothing here touches disk.
type Bus struct {
	em       Emitter
	logs     *LogBuffer
	queries  *Ring[engine.QueryEvent]
	conns    *Ring[proxy.ConnEvent]
	queryLog atomic.Bool
}

// NewBus creates a bus emitting through em.
func NewBus(em Emitter) *Bus {
	return &Bus{em: em, logs: NewLogBuffer(1000), queries: NewRing[engine.QueryEvent](500), conns: NewRing[proxy.ConnEvent](500)}
}

// ProxyConn receives every proxied connection; like queries it is kept
// (RAM only) and shown only while the query view is on.
func (b *Bus) ProxyConn(e proxy.ConnEvent) {
	if !b.queryLog.Load() {
		return
	}
	b.conns.Add(e)
	b.em.Emit(EventProxyConn, e)
}

// ProxyConns returns the RAM-only recent proxy connections.
func (b *Bus) ProxyConns() []proxy.ConnEvent { return b.conns.All() }

// State implements Sink.
func (b *Bus) State(s Snapshot) { b.em.Emit(EventState, s) }

// Log implements Sink.
func (b *Bus) Log(e LogEvent) {
	// File log: codes and params only (params never carry domain names).
	slog.Info("event", "source", e.Source, "code", e.Code, "params", e.Params)
	b.logs.Add(e)
	b.em.Emit(EventLog, e)
}

// Query receives every engine query; it is dropped unless the query view is on.
func (b *Bus) Query(q engine.QueryEvent) {
	if !b.queryLog.Load() {
		return
	}
	b.queries.Add(q)
	b.em.Emit(EventQuery, q)
}

// Emit forwards any other event.
func (b *Bus) Emit(name string, data any) { b.em.Emit(name, data) }
