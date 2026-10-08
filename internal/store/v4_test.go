package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSettings_V3toV4Defaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":3,"language":"en","dpi":{"engine":"goodbyedpi","preset":"medium"}}`), 0o644))
	s, recovered, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Equal(t, 5, s.Version)
	require.Equal(t, store.DNSServerSettings{DoHPort: 443}, s.DNSServer)
	require.Equal(t, store.FakeSNISettings{}, s.FakeSNI)
	require.Equal(t, store.EngineGoodbyeDPI, s.DPI.Engine)
	require.Equal(t, "medium", s.DPI.Preset)
	require.Equal(t, 5, store.DefaultSettings().Version)
}

func TestSettings_DNSServerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := store.DefaultSettings()
	s.DNSServer = store.DNSServerSettings{Enabled: true, ShareLAN: true, DoHPort: 8443, IOSSSID: "Nhà"}
	s.FakeSNI = store.FakeSNISettings{Enabled: true, AckVersion: 1}
	require.NoError(t, store.SaveSettings(path, s))
	got, _, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.Equal(t, s.DNSServer, got.DNSServer)
	require.Equal(t, s.FakeSNI, got.FakeSNI)
}

func TestSettings_DoHPortValidation(t *testing.T) {
	ok := store.DNSServerSettings{DoHPort: 443, IOSSSID: "home"}
	require.NoError(t, store.ValidateDNSServer(ok, 8080))
	for _, p := range []int{0, 53, 8053, 8080, 65536} {
		bad := ok
		bad.DoHPort = p
		require.Error(t, store.ValidateDNSServer(bad, 8080), p)
	}
	long := ok
	long.IOSSSID = "123456789012345678901234567890123"
	require.Error(t, store.ValidateDNSServer(long, 8080))
}

func TestState_V2FirewallRuleUpgrades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":2,"phase":"dns_set","firewall":{"rule":"Ghostline Proxy"}}`), 0o644))
	st, err := store.NewStateStore(path, &fakeLocker{}).Load()
	require.NoError(t, err)
	require.Equal(t, []string{"Ghostline Proxy"}, st.Firewall.Rules)
	require.Nil(t, st.Certs)
}

func TestState_V3RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := store.NewStateStore(path, &fakeLocker{})
	require.NoError(t, s.Update(func(st *store.State) error {
		st.AddFirewallRule("Ghostline DNS (TCP)")
		st.AddFirewallRule("Ghostline DNS (UDP)")
		st.AddFirewallRule("Ghostline DNS (TCP)")
		st.AddSessionCert("aa")
		return nil
	}))
	st, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, 4, st.Version)
	require.Equal(t, []string{"Ghostline DNS (TCP)", "Ghostline DNS (UDP)"}, st.Firewall.Rules)
	require.Equal(t, []string{"aa"}, st.Certs.Session)

	st.RemoveFirewallRule("Ghostline DNS (TCP)")
	st.RemoveFirewallRule("Ghostline DNS (UDP)")
	st.RemoveSessionCert("aa")
	require.Nil(t, st.Firewall)
	require.Nil(t, st.Certs)
	require.Equal(t, 4, store.CleanState().Version)
}
