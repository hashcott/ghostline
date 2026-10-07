package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/model"
)

// Phase is the connection phase persisted in state.json.
type Phase string

const (
	PhaseClean  Phase = "clean"
	PhaseDNSSet Phase = "dns_set"
)

// ErrStateCorrupt means state.json exists but cannot be parsed.
var ErrStateCorrupt = errors.New("store: state.json is corrupt")

// DPIState records a running DPI engine process.
type DPIState struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid"`
	Engine  string `json:"engine,omitempty"`
}

// State is the write-ahead record of what Ghostline changed on the system.
type State struct {
	Version      int               `json:"version"`
	Phase        Phase             `json:"phase"`
	PID          uint32            `json:"pid"`
	PIDStartTime time.Time         `json:"pidStartTime"`
	StartedAt    time.Time         `json:"startedAt"`
	DNS          model.DNSSnapshot `json:"dns"`
	DPI          DPIState          `json:"dpi"`
	SysProxy     *SysProxyState    `json:"sysproxy,omitempty"`
	Firewall     *FirewallState    `json:"firewall,omitempty"`
	Certs        *CertsState       `json:"certs,omitempty"`
}

// UnmarshalJSON reads v4, and v3 files (v0.5), whose adapters were in
// "snapshot": they become a "windows" DNS snapshot.
func (s *State) UnmarshalJSON(b []byte) error {
	type plain State
	aux := struct {
		*plain
		Legacy []model.AdapterSnapshot `json:"snapshot"`
	}{plain: (*plain)(s)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if len(aux.Legacy) > 0 && s.DNS.Empty() {
		s.DNS = model.DNSSnapshot{Backend: "windows", Windows: aux.Legacy}
	}
	if s.Version < 4 {
		s.Version = 4
	}
	return nil
}

// SysProxySnapshot is the WinINET per-connection proxy configuration.
type SysProxySnapshot struct {
	Flags         uint32 `json:"flags"`
	Server        string `json:"server"`
	Bypass        string `json:"bypass"`
	AutoconfigURL string `json:"autoconfigUrl"`
}

// SysProxyState records Ghostline's change to the system proxy. Set is true
// once Ghostline applied Ours; TakenOver once another app replaced it.
type SysProxyState struct {
	Set       bool              `json:"set"`
	TakenOver bool              `json:"takenOver"`
	Ours      string            `json:"ours"`
	Snapshot  *SysProxySnapshot `json:"snapshot"`
}

// FirewallState records the inbound rules Ghostline created, by name.
type FirewallState struct {
	Rules []string `json:"rules"`
}

// UnmarshalJSON also reads the v2 form {"rule": "<name>"}.
func (f *FirewallState) UnmarshalJSON(b []byte) error {
	var v struct {
		Rules []string `json:"rules"`
		Rule  string   `json:"rule"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	f.Rules = v.Rules
	if v.Rule != "" && !slices.Contains(f.Rules, v.Rule) {
		f.Rules = append(f.Rules, v.Rule)
	}
	return nil
}

// CertsState records the Fake SNI session CAs installed in the Root
// store (SHA-1 thumbprints), written before each install.
type CertsState struct {
	Session []string `json:"session"`
}

// AddFirewallRule records a rule name (once).
func (s *State) AddFirewallRule(name string) {
	if s.Firewall == nil {
		s.Firewall = &FirewallState{}
	}
	if !slices.Contains(s.Firewall.Rules, name) {
		s.Firewall.Rules = append(s.Firewall.Rules, name)
	}
}

// RemoveFirewallRule forgets a rule name; no rule left clears the field.
func (s *State) RemoveFirewallRule(name string) {
	if s.Firewall == nil {
		return
	}
	s.Firewall.Rules = slices.DeleteFunc(s.Firewall.Rules, func(x string) bool { return x == name })
	if len(s.Firewall.Rules) == 0 {
		s.Firewall = nil
	}
}

// AddSessionCert records a session CA thumbprint (once).
func (s *State) AddSessionCert(thumbprint string) {
	if s.Certs == nil {
		s.Certs = &CertsState{}
	}
	if !slices.Contains(s.Certs.Session, thumbprint) {
		s.Certs.Session = append(s.Certs.Session, thumbprint)
	}
}

// RemoveSessionCert forgets a thumbprint; none left clears the field.
func (s *State) RemoveSessionCert(thumbprint string) {
	if s.Certs == nil {
		return
	}
	s.Certs.Session = slices.DeleteFunc(s.Certs.Session, func(x string) bool { return x == thumbprint })
	if len(s.Certs.Session) == 0 {
		s.Certs = nil
	}
}

// CleanState is the state with nothing to restore.
func CleanState() State { return State{Version: 4, Phase: PhaseClean} }

func cleanState() State { return CleanState() }

// Locker serialises access to state.json across processes.
type Locker interface {
	Lock() error
	Unlock() error
}

// StateStore reads and writes state.json under a cross-process lock.
type StateStore struct {
	path string
	lock Locker
}

func NewStateStore(path string, l Locker) *StateStore { return &StateStore{path: path, lock: l} }

// Path is the state file location.
func (s *StateStore) Path() string { return s.path }

// Locked runs fn while holding the cross-process lock, for read-decide-write
// sequences that span more than one Update (e.g. restore after corruption).
func (s *StateStore) Locked(fn func() error) error {
	if err := s.lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = s.lock.Unlock() }()
	return fn()
}

// Write replaces state.json; callers must hold the lock (see Locked).
func (s *StateStore) Write(st State) error { return WriteJSONAtomic(s.path, st) }

// Load reads state.json; a missing file is a clean state.
func (s *StateStore) Load() (State, error) {
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return cleanState(), nil
	}
	if err != nil {
		return State{}, err
	}
	st := cleanState()
	if err := json.Unmarshal(b, &st); err != nil {
		return State{}, fmt.Errorf("%w: %v", ErrStateCorrupt, err)
	}
	return st, nil
}

// Update loads the state, applies fn and writes it back, all under the lock.
// If fn returns an error nothing is written.
func (s *StateStore) Update(fn func(*State) error) error {
	if err := s.lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = s.lock.Unlock() }()
	st, err := s.Load()
	if err != nil {
		return err
	}
	if err := fn(&st); err != nil {
		return err
	}
	return WriteJSONAtomic(s.path, st)
}

// Reset overwrites state.json with a clean state, under the lock. It is the
// way out of a corrupt state file.
func (s *StateStore) Reset() error {
	if err := s.lock.Lock(); err != nil {
		return err
	}
	defer func() { _ = s.lock.Unlock() }()
	return WriteJSONAtomic(s.path, cleanState())
}
