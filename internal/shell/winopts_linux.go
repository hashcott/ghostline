package shell

import (
	"github.com/hashcott/ghostline/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// applyPlatformOptions: closing the window quits the GUI on Linux. Not every
// desktop shows a tray (GNOME), and without one a hidden window could not be
// brought back; the daemon (L2) keeps protection running instead.
func applyPlatformOptions(*application.Options, *app.Orchestrator) {}
