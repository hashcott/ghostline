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
