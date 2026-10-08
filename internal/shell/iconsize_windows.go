package shell

import "github.com/hashcott/ghostline/internal/winutil"

// trayIconSize is the system's small icon size in pixels.
func trayIconSize() int { return winutil.SmallIconSize() }
