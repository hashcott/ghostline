// Package app orchestrates Ghostline: connect, disconnect, recovery, health
// and the service bound to the UI.
package app

import (
	"maps"
	"time"
)

// Status is the connection state shown to the user.
type Status string

const (
	StatusDisconnected  Status = "disconnected"
	StatusConnecting    Status = "connecting"
	StatusProtected     Status = "protected"
	StatusDegraded      Status = "degraded"
	StatusDisconnecting Status = "disconnecting"
	StatusError         Status = "error"
)

// DPIStatus summarises the DPI engine for the UI.
type DPIStatus struct {
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Engine   string `json:"engine"`   // engine actually running, "" when stopped
	Preset   string `json:"preset"`   // strategy/preset actually running
	Fallback bool   `json:"fallback"` // GoodbyeDPI runs because zapret2 could not
}

// ReasonDPIFallback marks the connection degraded while GoodbyeDPI stands
// in for a blocked zapret2.
const ReasonDPIFallback = "dpiFallback"

// ProxyStatus summarises the local proxy for the UI.
// DNSServerStatus is the DNS server phase (spec 2B 6.1).
type DNSServerStatus struct {
	Running bool              `json:"running"`
	Addrs   []string          `json:"addrs"`
	Skipped map[string]string `json:"skipped,omitempty"` // address → bind error
	Error   *AppError         `json:"error,omitempty"`
}

// FakeSNIStatus is the Fake SNI phase (spec 2B 6.2).
type FakeSNIStatus struct {
	Active     bool      `json:"active"`
	Domains    int       `json:"domains"`
	Thumbprint string    `json:"thumbprint,omitempty"`
	NotAfter   time.Time `json:"notAfter"`
	NeedsProxy bool      `json:"needsProxy"`
	Error      *AppError `json:"error,omitempty"`
}

type ProxyStatus struct {
	Running     bool      `json:"running"`
	Addr        string    `json:"addr"`
	SystemProxy bool      `json:"systemProxy"`
	ShareLAN    bool      `json:"shareLan"`
	Error       *AppError `json:"error,omitempty"`
}

// Snapshot is the UI-facing state, emitted on every change.
type Snapshot struct {
	Platform     string     `json:"platform"` // "windows", "linux": the UI's wording
	Status       Status     `json:"status"`
	Step         int        `json:"step"`                // 1..7 while connecting
	PickDone     int        `json:"pickDone,omitempty"`  // servers checked so far in step 2
	PickTotal    int        `json:"pickTotal,omitempty"` // servers to check in step 2 (0 = none)
	Error        *AppError  `json:"error,omitempty"`
	Warnings     []AppError `json:"warnings"`
	Servers      []string   `json:"servers"`
	Since        time.Time  `json:"since"`
	LatencyMs    int        `json:"latencyMs"`
	Queries      uint64     `json:"queries"`
	DPI          DPIStatus  `json:"dpi"`
	BlockedSites []string   `json:"blockedSites"`
	// Probed: the test sites were checked after connecting (DPI bypass
	// already running), so BlockedSites is final.
	Probed bool `json:"probed"`
	// Reasons lists why the connection is degraded: "upstreams", "proxy".
	Reasons   []string        `json:"reasons"`
	Proxy     ProxyStatus     `json:"proxy"`
	DNSServer DNSServerStatus `json:"dnsServer"`
	FakeSNI   FakeSNIStatus   `json:"fakeSni"`
}

func (s Snapshot) clone() Snapshot {
	c := s
	c.Warnings = append([]AppError(nil), s.Warnings...)
	c.Servers = append([]string(nil), s.Servers...)
	c.BlockedSites = append([]string(nil), s.BlockedSites...)
	c.Reasons = append([]string(nil), s.Reasons...)
	if s.Proxy.Error != nil {
		e := *s.Proxy.Error
		c.Proxy.Error = &e
	}
	if s.Error != nil {
		e := *s.Error
		c.Error = &e
	}
	if s.FakeSNI.Error != nil {
		e := *s.FakeSNI.Error
		c.FakeSNI.Error = &e
	}
	c.DNSServer.Addrs = append([]string(nil), s.DNSServer.Addrs...)
	c.DNSServer.Skipped = maps.Clone(s.DNSServer.Skipped)
	if s.DNSServer.Error != nil {
		e := *s.DNSServer.Error
		c.DNSServer.Error = &e
	}
	return c
}

// LogEvent is a structured, translatable log line for the UI.
type LogEvent struct {
	Time   time.Time      `json:"time"`
	Source string         `json:"source"` // engine | dpi | system | ok
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
	// Err is the underlying error, for the log file only: the UI shows the
	// translated code, and the cause may name things it should not.
	Err string `json:"-"`
}
