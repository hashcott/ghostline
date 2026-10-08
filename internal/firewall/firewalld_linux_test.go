package firewall

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeFirewalld struct {
	ports  map[string]bool
	ifZone string
	def    string
}

func (f *fakeFirewalld) QueryPort(zone, port, proto string) (bool, error) {
	return f.ports[zone+" "+port+"/"+proto], nil
}
func (f *fakeFirewalld) AddPort(zone, port, proto string) error {
	f.ports[zone+" "+port+"/"+proto] = true
	return nil
}
func (f *fakeFirewalld) RemovePort(zone, port, proto string) error {
	delete(f.ports, zone+" "+port+"/"+proto)
	return nil
}
func (f *fakeFirewalld) ZoneOfInterface(string) (string, error) { return f.ifZone, nil }
func (f *fakeFirewalld) DefaultZone() (string, error)           { return f.def, nil }

func TestFirewalld_ZoneAndPublic(t *testing.T) {
	api := &fakeFirewalld{ports: map[string]bool{}, ifZone: "public", def: "home"}
	m := newLinux(filepath.Join(t.TempDir(), "f.json"), firewalld{api: api, iface: func() string { return "wlan0" }})
	require.NoError(t, m.Add(8080))
	require.True(t, api.ports["public 8080/tcp"], "the LAN interface's zone")
	pub, err := m.IsPublicNetwork()
	require.NoError(t, err)
	require.True(t, pub)
	require.NoError(t, m.Delete())
	require.Empty(t, api.ports)

	api.ifZone = "" // interface in no zone: the default zone applies
	z, err := firewalld{api: api, iface: func() string { return "wlan0" }}.zone()
	require.NoError(t, err)
	require.Equal(t, "home", z)
	pub, _ = firewalld{api: api, iface: func() string { return "wlan0" }}.public()
	require.False(t, pub)
}
