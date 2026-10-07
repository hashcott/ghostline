package shell

import (
	"time"

	"github.com/hashcott/ghostline/internal/startup"
)

// safety implements app.Safety.
type safety struct {
	startup       startup.Manager
	startWatchdog func(pid uint32, start time.Time) (stop func() error, err error)
}

func (s safety) StartWatchdog(pid uint32, start time.Time) (func() error, error) {
	return s.startWatchdog(pid, start)
}

func (s safety) CreateRecoveryTask() error { return s.startup.CreateRecovery() }
func (s safety) DeleteRecoveryTask() error { return s.startup.DeleteRecovery() }
