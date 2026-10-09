package procs

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var in Inspector = Unsupported{}
	require.False(t, in.IsAdmin())
	_, err := in.PortOwners(53)
	require.ErrorIs(t, err, errors.ErrUnsupported)
	_, err = in.StartTime(1)
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, in.WaitForExit(1), errors.ErrUnsupported)
	require.ErrorIs(t, in.StopService("x", time.Second), errors.ErrUnsupported)
	require.False(t, in.Alive(1, time.Time{}))
	_, err = in.ProcessNames()
	require.ErrorIs(t, err, errors.ErrUnsupported)
}
