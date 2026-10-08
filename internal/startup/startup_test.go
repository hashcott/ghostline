package startup_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/startup"
	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var m startup.Manager = startup.Unsupported{}
	require.NoError(t, m.SetAutostart(false))
	require.ErrorIs(t, m.SetAutostart(true), errors.ErrUnsupported)
	require.ErrorIs(t, m.CreateRecovery(), errors.ErrUnsupported)
	require.NoError(t, m.DeleteRecovery())
}

// A service manager (systemd) runs the daemon: it is the recovery step
// itself and starts the daemon at boot; the GUI writes its own autostart
// entry, so there is nothing to register here.
func TestServiceManaged(t *testing.T) {
	var m startup.Manager = startup.ServiceManaged{}
	require.NoError(t, m.CreateRecovery())
	require.NoError(t, m.DeleteRecovery())
	require.NoError(t, m.SetAutostart(false))
	require.NoError(t, m.SetAutostart(true))
}
