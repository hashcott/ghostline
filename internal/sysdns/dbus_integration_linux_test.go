//go:build integration

package sysdns

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDBus_ReadsNMWhenPresent(t *testing.T) {
	d, err := newDBus()
	if err != nil {
		t.Skip("no system bus:", err)
	}
	if !d.NM.Running() {
		t.Skip("NetworkManager not running")
	}
	_, err = d.NM.GlobalDNS()
	require.NoError(t, err)
	mode, err := d.NM.DNSMode()
	require.NoError(t, err)
	require.NotEmpty(t, mode)
	_, err = d.NM.DNSServers()
	require.NoError(t, err)
	if d.Resolved.Running() {
		_, err = d.Resolved.Links()
		require.NoError(t, err)
	}
}
