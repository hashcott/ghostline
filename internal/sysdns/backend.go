package sysdns

import "github.com/hashcott/ghostline/internal/model"

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
