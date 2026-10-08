package procs

import (
	"time"

	"github.com/hashcott/ghostline/internal/winutil"
)

type windowsInspector struct{}

// NewWindows inspects processes through the Win32 API and the SCM.
func NewWindows() Inspector { return windowsInspector{} }

func (windowsInspector) IsAdmin() bool { return winutil.IsAdmin() }

func (windowsInspector) PortOwners(port uint16) ([]PortOwner, error) {
	ws, err := winutil.PortOwners(port)
	if err != nil {
		return nil, err
	}
	out := make([]PortOwner, len(ws))
	for i, w := range ws {
		out[i] = PortOwner{PID: w.PID, Name: w.Name, Service: w.Service, Proto: w.Proto}
	}
	return out, nil
}

func (windowsInspector) StartTime(pid uint32) (time.Time, error) {
	return winutil.ProcessStartTime(pid)
}
func (windowsInspector) Alive(pid uint32, start time.Time) bool {
	return winutil.ProcessAlive(pid, start)
}
func (windowsInspector) WaitForExit(pid uint32) error { return winutil.WaitForExit(pid) }
func (windowsInspector) StopService(name string, wait time.Duration) error {
	return winutil.StopService(name, wait)
}
