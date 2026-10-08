package sysdns_test

import (
	"errors"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

func TestUnsupported(t *testing.T) {
	var api sysdns.API = sysdns.Unsupported{}
	_, err := api.Adapters()
	require.ErrorIs(t, err, errors.ErrUnsupported)
	_, err = api.GetDNS("g", false)
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.ErrorIs(t, api.SetDNS("g", false, nil), errors.ErrUnsupported)
	require.ErrorIs(t, api.NetshSetDNS(1, false, nil), errors.ErrUnsupported)
	require.NoError(t, api.Flush())
}

// Review Focus 5: Connect on an unsupported OS stops at the first DNS step,
// and a restore with nothing recorded succeeds.
func TestManagerOverUnsupported_FailsBeforeAnyChange(t *testing.T) {
	m := sysdns.NewManager(sysdns.Unsupported{}, func(time.Duration) {})
	_, err := m.Select("auto", nil)
	require.ErrorIs(t, err, errors.ErrUnsupported)
	require.Empty(t, m.Restore(nil))
	require.NoError(t, m.Flush())
}
