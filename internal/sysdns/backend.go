package sysdns

import (
	"fmt"
	"strings"

	"github.com/hashcott/ghostline/internal/model"
)

// Snapshot is what a backend records before changing DNS (state.json).
type Snapshot = model.DNSSnapshot

// Selection is the user's adapter choice; backends without adapters
// ignore it.
type Selection struct {
	Mode string   // "auto" | "manual"
	IDs  []string // manual choice
}

// Change is one thing Reconcile touched: an adapter added (Windows) or a
// configuration set again after the system changed it (Linux).
type Change struct {
	Target string
	Added  bool
}

// Info describes the backend for the Settings page.
type Info struct {
	Backend     string    `json:"backend"`
	Chain       string    `json:"chain"`
	Interfaces  []string  `json:"interfaces"`
	AdapterPick bool      `json:"adapterPick"`
	Adapters    []Adapter `json:"adapters"`
}

// Backend changes the system's DNS through one mechanism of one OS. The
// orchestrator, the watchdog and recovery use only this.
type Backend interface {
	Name() string
	// Snapshot records what Apply will change.
	Snapshot(sel Selection) (Snapshot, error)
	// Apply points the recorded configuration at 127.0.0.1 (and ::1).
	Apply(s Snapshot, v6 bool) error
	// Reconcile runs after a network change and changes nothing: it returns
	// next (s plus whatever is new, recorded as it is now), toApply (the
	// part to point at loopback again) and what that covers. The caller
	// persists next before it applies toApply, so a crash in between never
	// leaves an unrecorded change.
	Reconcile(s Snapshot, sel Selection) (next, toApply Snapshot, changes []Change, err error)
	// StillOurs keeps the part of s Ghostline's configuration still holds,
	// so recovery never overwrites what the user set since.
	StillOurs(s Snapshot) Snapshot
	Restore(s Snapshot) []RestoreError
	// RestoreDefault undoes Ghostline's configuration without a snapshot.
	RestoreDefault() error
	Flush() error
	// Watch reports network or DNS configuration changes.
	Watch(onChange func()) (stop func(), err error)
	Info() Info
}

// ApplyError is an Apply that set some targets and failed on others.
// Failed names those as Change.Target does.
type ApplyError struct {
	Failed []string
	Err    error
}

func (e *ApplyError) Error() string {
	return fmt.Sprintf("sysdns: could not set %s: %v", strings.Join(e.Failed, ", "), e.Err)
}

func (e *ApplyError) Unwrap() error { return e.Err }

// ListAdapters lists the adapters a user can pick from, with the error
// Info leaves out; a backend without adapters has none.
func ListAdapters(b Backend) ([]Adapter, error) {
	if l, ok := b.(interface{ Adapters() ([]Adapter, error) }); ok {
		return l.Adapters()
	}
	return b.Info().Adapters, nil
}
