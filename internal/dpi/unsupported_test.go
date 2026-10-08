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

	var ic Interceptor = NoInterceptor{}
	require.NoError(t, ic.Prepare(t.TempDir()))
	require.True(t, ic.Ready(1))
	require.NoError(t, ic.Cleanup())
	require.Equal(t, InterceptorInfo{}, ic.Info())
}
