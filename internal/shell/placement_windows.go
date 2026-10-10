package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// windowScale: on Windows the window's size and position share one unit.
func windowScale(*application.WebviewWindow) float64 { return 1 }

// centre: Wails already centres a new window correctly on Windows.
func centre(*application.WebviewWindow) {}
