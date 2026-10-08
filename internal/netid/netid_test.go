package netid

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var s Source = Unsupported{}
	require.Equal(t, scanner.NetworkKey("none", "none"), s.NetworkKey())
	_, err := s.CurrentSSID()
	require.ErrorIs(t, err, errors.ErrUnsupported)
	_, err = s.WifiNames()
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.Empty(t, s.LiveAdapters())
}
