package sysdns

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestSettingsStructSize(t *testing.T) {
	require.Equal(t, uintptr(64), unsafe.Sizeof(dnsInterfaceSettings{}))
}

func TestWindowsAPI_AdaptersHaveGUIDs(t *testing.T) {
	ads, err := NewWindowsAPI().Adapters()
	require.NoError(t, err)
	require.NotEmpty(t, ads)
	for _, a := range ads {
		_, err := windows.GUIDFromString(a.GUID)
		require.NoError(t, err, a.GUID)
		require.NotZero(t, a.IfIndex)
	}
}

func TestWindowsAPI_GetDNSReadOnly(t *testing.T) {
	api := NewWindowsAPI()
	ads, err := api.Adapters()
	require.NoError(t, err)
	for _, a := range ads {
		_, err := api.GetDNS(a.GUID, false)
		require.NoError(t, err, a.Alias)
	}
}

// The registry holds what GetInterfaceDnsSettings returns, so Windows
// without that call reads the same servers.
func TestRegistryDNS_MatchesGetInterfaceDnsSettings(t *testing.T) {
	api := NewWindowsAPI()
	ads, err := api.Adapters()
	require.NoError(t, err)
	for _, a := range ads {
		for _, v6 := range []bool{false, true} {
			want, err := api.GetDNS(a.GUID, v6)
			if err != nil {
				continue // e.g. IPv6 of an adapter without IPv6: nothing to compare
			}
			got, err := registryDNS(a.GUID, v6)
			require.NoError(t, err, a.Alias)
			require.ElementsMatch(t, want, got, "%s v6=%v", a.Alias, v6)
		}
	}
}

func TestWindowsAPI_WithoutDNSSettingsAPI(t *testing.T) {
	old := haveDNSSettingsAPI
	haveDNSSettingsAPI = func() bool { return false }
	t.Cleanup(func() { haveDNSSettingsAPI = old })
	api := NewWindowsAPI()
	ads, err := api.Adapters()
	require.NoError(t, err)
	require.NotEmpty(t, ads)
	_, err = api.GetDNS(ads[0].GUID, false) // from the registry, no panic
	require.NoError(t, err)
	// Refused before touching anything: the Manager falls back to netsh.
	require.ErrorIs(t, api.SetDNS(ads[0].GUID, false, []string{"127.0.0.1"}), errNoDNSSettingsAPI)
}

func TestRegistryDNS_UnknownAdapterIsDHCP(t *testing.T) {
	got, err := registryDNS("{00000000-0000-0000-0000-000000000001}", false)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestSplitNameServers(t *testing.T) {
	require.Equal(t, []string{"1.1.1.1", "8.8.8.8"}, splitNameServers("1.1.1.1,8.8.8.8"))
	require.Equal(t, []string{"1.1.1.1", "8.8.8.8"}, splitNameServers(" 1.1.1.1 8.8.8.8 "))
	require.Empty(t, splitNameServers(""))
}

func TestNetshArgs(t *testing.T) {
	require.Equal(t, []string{"interface", "ipv4", "set", "dnsservers", "name=12", "source=static", "address=127.0.0.1", "register=primary", "validate=no"},
		netshArgs(12, false, []string{"127.0.0.1"}))
	require.Equal(t, []string{"interface", "ipv6", "set", "dnsservers", "name=12", "source=dhcp"}, netshArgs(12, true, nil))
}

func TestWatch_StartsAndStops(t *testing.T) {
	stop, err := Watch(func() {})
	require.NoError(t, err)
	stop()
}
