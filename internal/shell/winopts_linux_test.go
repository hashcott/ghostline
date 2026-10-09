package shell

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Closing the window to the tray destroys it; Wails must not then quit the
// GUI because no window is left. The WindowClosing hook quits when the
// setting is off.
func TestApplyPlatformOptions_KeepsRunningWithoutWindow(t *testing.T) {
	var opts application.Options
	applyPlatformOptions(&opts, nil)
	require.True(t, opts.Linux.DisableQuitOnLastWindowClosed)
}
