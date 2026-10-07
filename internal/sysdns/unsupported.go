package sysdns

import (
	"errors"
	"fmt"
)

// Unsupported is the API for an OS without support yet: changing DNS
// fails, and there is no cache to flush.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("sysdns: %w", errors.ErrUnsupported)

func (Unsupported) Adapters() ([]Adapter, error)             { return nil, errUnsupported }
func (Unsupported) GetDNS(string, bool) ([]string, error)    { return nil, errUnsupported }
func (Unsupported) SetDNS(string, bool, []string) error      { return errUnsupported }
func (Unsupported) NetshSetDNS(uint32, bool, []string) error { return errUnsupported }
func (Unsupported) Flush() error                             { return nil }

// UnsupportedBackend is the Backend for an OS without system DNS support
// yet: changing DNS fails; restoring has nothing to undo.
type UnsupportedBackend struct{}

func (UnsupportedBackend) Name() string                         { return "unsupported" }
func (UnsupportedBackend) Snapshot(Selection) (Snapshot, error) { return Snapshot{}, errUnsupported }
func (UnsupportedBackend) Apply(Snapshot, bool) error           { return errUnsupported }
func (UnsupportedBackend) Reconcile(s Snapshot, _ Selection, _ bool) (Snapshot, []Change, error) {
	return s, nil, nil
}
func (UnsupportedBackend) StillOurs(Snapshot) Snapshot     { return Snapshot{} }
func (UnsupportedBackend) Restore(Snapshot) []RestoreError { return nil }
func (UnsupportedBackend) RestoreDefault() error           { return errUnsupported }
func (UnsupportedBackend) Flush() error                    { return nil }
func (UnsupportedBackend) Watch(func()) (func(), error)    { return func() {}, nil }
func (UnsupportedBackend) Info() Info                      { return Info{Backend: "unsupported", Interfaces: []string{}} }
