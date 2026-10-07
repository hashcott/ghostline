package dpi

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var r Runner = UnsupportedRunner{}
	_, err := r.Start("x", nil, "")
	require.ErrorIs(t, err, errors.ErrUnsupported)

	var s Services = NoServices{}
	names, err := s.Find("WinDivert")
	require.NoError(t, err)
	require.Empty(t, names)
	running, err := s.Running("WinDivert")
	require.NoError(t, err)
	require.False(t, running)
	require.NoError(t, s.Stop("WinDivert"))
	require.NoError(t, s.Delete("WinDivert"))
}
