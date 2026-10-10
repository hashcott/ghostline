package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// windowScale is the scale of the window's monitor: Wails on GTK4 sizes
// windows in logical pixels but positions them in device (X11) pixels.
func windowScale(w *application.WebviewWindow) float64 {
	s, err := w.GetScreen()
	if err != nil {
		return 1
	}
	return screenScale(s)
}

func screenScale(s *application.Screen) float64 {
	if s == nil || s.ScaleFactor <= 0 {
		return 1
	}
	return float64(s.ScaleFactor)
}

// centre puts a window just shown in the middle of its monitor. Wails
// centres it from the monitor's logical geometry but moves it in device
// pixels, so with display scaling it lands up and left of centre, partly
// off-screen when the monitor does not start at 0,0. Wayland does not let
// a window move itself; there this does nothing.
func centre(w *application.WebviewWindow) {
	s, err := w.GetScreen()
	if err != nil || s == nil {
		return
	}
	ww, wh := w.Size()
	w.SetPosition(centredOn(s.PhysicalBounds, screenScale(s), ww, wh))
}
