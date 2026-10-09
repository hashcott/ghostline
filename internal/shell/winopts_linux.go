package shell

import (
	"github.com/hashcott/ghostline/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// applyPlatformOptions keeps the GUI running when its window closes to the
// tray; the WindowClosing hook quits when "close to tray" is off. Where the
// desktop shows no tray (GNOME without an extension), opening Ghostline
// again brings the window back through the single-instance handler.
func applyPlatformOptions(opts *application.Options, _ *app.Orchestrator) {
	opts.Linux.DisableQuitOnLastWindowClosed = true
}
