package shell

import (
	"math"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Window sizes are in logical pixels; window positions and monitor bounds
// are in device pixels. scale (the monitor's) converts one to the other.

// centredOn returns the position that centres a w×h window on mon.
func centredOn(mon application.Rect, scale float64, w, h int) (x, y int) {
	return mon.X + (mon.Width-devicePx(w, scale))/2, mon.Y + (mon.Height-devicePx(h, scale))/2
}

// recentred returns the position that keeps the centre of a window at x,y
// in place when it goes from w0×h0 to w×h.
func recentred(x, y, w0, h0, w, h int, scale float64) (int, int) {
	return x + (devicePx(w0, scale)-devicePx(w, scale))/2, y + (devicePx(h0, scale)-devicePx(h, scale))/2
}

func devicePx(n int, scale float64) int { return int(math.Round(float64(n) * scale)) }
