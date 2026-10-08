package sysproxy_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var b sysproxy.Backend = sysproxy.Unsupported{}
	_, err := b.Snapshot("127.0.0.1:8080")
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, b.Apply("127.0.0.1:8080"), errors.ErrUnsupported)
	_, err = b.Watch(func() {})
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.Equal(t, sysproxy.Info{}, b.Info())
}
