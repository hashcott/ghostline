package netwatch

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCombine_StopsStartedOnError(t *testing.T) {
	stopped := 0
	ok := func(func()) (func(), error) { return func() { stopped++ }, nil }
	bad := func(func()) (func(), error) { return nil, errors.New("no bus") }
	_, err := Combine(ok, ok, bad)(func() {})
	require.Error(t, err)
	require.Equal(t, 2, stopped)

	stopped = 0
	stop, err := Combine(ok, ok)(func() {})
	require.NoError(t, err)
	stop()
	require.Equal(t, 2, stopped)
}
