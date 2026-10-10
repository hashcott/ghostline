package shell

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// A 4096x2304 monitor at y=1238 with 2x scaling (the reporter's Linux Mint
// desktop): the window size is in logical pixels, the monitor in device
// pixels. Mixing the two put the window above the monitor.
func TestCentredOn_ScaledMonitorOffOrigin(t *testing.T) {
	mon := application.Rect{X: 0, Y: 1238, Width: 4096, Height: 2304}
	x, y := centredOn(mon, 2, 1345, 660)
	require.Equal(t, 703, x)
	require.Equal(t, 1730, y)
}

func TestCentredOn_Unscaled(t *testing.T) {
	x, y := centredOn(application.Rect{X: 1920, Y: 0, Width: 1920, Height: 1080}, 1, 380, 580)
	require.Equal(t, 1920+770, x)
	require.Equal(t, 250, y)
}

// Switching modes keeps the window's centre: its position is in device
// pixels, its sizes in logical ones.
func TestRecentred_KeepsCentreWhenScaled(t *testing.T) {
	x, y := recentred(703, 1730, 1345, 660, 380, 580, 2) // centre (2048, 2390)
	require.Equal(t, 2048-380, x)
	require.Equal(t, 2390-580, y)
}

func TestRecentred_Unscaled(t *testing.T) {
	x, y := recentred(100, 100, 380, 580, 900, 600, 1) // centre (290, 390)
	require.Equal(t, 290-450, x)
	require.Equal(t, 390-300, y)
}
