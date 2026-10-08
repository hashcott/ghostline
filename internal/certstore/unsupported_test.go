package certstore_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var s certstore.Store = certstore.Unsupported{}
	require.ErrorIs(t, s.Install([]byte{1}), errors.ErrUnsupported)
	require.NoError(t, s.Remove("ab"))
	certs, err := s.List("Ghostline")
	require.NoError(t, err)
	require.Empty(t, certs)
}
