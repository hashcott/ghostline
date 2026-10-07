package sysproxy_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var api sysproxy.API = sysproxy.Unsupported{}
	_, err := api.Query()
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, api.Set(store.SysProxySnapshot{}), errors.ErrUnsupported)
}
