package shell

type wmAction int

const (
	wmNone wmAction = iota
	wmEndSession
	wmResume
)

const (
	wmQueryEndSession     = 0x11
	wmEndSessionMsg       = 0x16
	wmPowerBroadcast      = 0x218
	pbtAPMResumeAutomatic = 0x12
)

// classify maps a window message to the action Ghostline takes:
// restore DNS before Windows logs off or shuts down, and re-check the
// engine after sleep.
func classify(msg uint32, wParam uintptr) wmAction {
	switch msg {
	case wmQueryEndSession:
		return wmEndSession
	case wmEndSessionMsg:
		if wParam != 0 {
			return wmEndSession
		}
	case wmPowerBroadcast:
		if wParam == pbtAPMResumeAutomatic {
			return wmResume
		}
	}
	return wmNone
}
