// Package startup registers what runs at logon or boot: Ghostline itself and
// the DNS recovery step (Task Scheduler tasks on Windows).
package startup

import (
	"errors"
	"fmt"
)

// Manager registers what runs at logon or boot: Ghostline itself (when the
// user asks to start with the system) and the DNS recovery step.
type Manager interface {
	SetAutostart(on bool) error
	CreateRecovery() error
	DeleteRecovery() error
}

// Unsupported is the Manager for an OS without support yet: registering
// fails, unregistering has nothing to undo.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("startup: %w", errors.ErrUnsupported)

func (Unsupported) SetAutostart(on bool) error {
	if on {
		return errUnsupported
	}
	return nil
}
func (Unsupported) CreateRecovery() error { return errUnsupported }
func (Unsupported) DeleteRecovery() error { return nil }

// ServiceManaged is the Manager where a service manager runs Ghostline
// (systemd's ghostline.service on Linux): the unit itself is the recovery
// step (ExecStopPost=--restore, and the daemon restores first at boot), so
// there is nothing to register. Starting the GUI at logon is not done yet.
type ServiceManaged struct{}

func (ServiceManaged) SetAutostart(on bool) error { return Unsupported{}.SetAutostart(on) }
func (ServiceManaged) CreateRecovery() error      { return nil }
func (ServiceManaged) DeleteRecovery() error      { return nil }
