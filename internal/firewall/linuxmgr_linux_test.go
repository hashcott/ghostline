package firewall

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeBackend is a port table; pre lists ports open before Ghostline.
type fakeBackend struct {
	open map[string]bool
	log  []string
	zn   string
	pub  bool
}

func key(zone, proto string, port int) string { return fmt.Sprintf("%s %d/%s", zone, port, proto) }

func (f *fakeBackend) exists(zone, proto string, port int) (bool, error) {
	return f.open[key(zone, proto, port)], nil
}
func (f *fakeBackend) add(zone, proto string, port int, name string) error {
	f.log = append(f.log, "open "+key(zone, proto, port)+" "+name)
	f.open[key(zone, proto, port)] = true
	return nil
}
func (f *fakeBackend) remove(zone, proto string, port int) error {
	f.log = append(f.log, "close "+key(zone, proto, port))
	delete(f.open, key(zone, proto, port))
	return nil
}
func (f *fakeBackend) zone() (string, error) { return f.zn, nil }
func (f *fakeBackend) public() (bool, error) { return f.pub, nil }

func newFake() *fakeBackend { return &fakeBackend{open: map[string]bool{}, zn: "home"} }

// A crash between opening and --restore: the rule's ports come from the
// file, not from memory.
func TestLinuxFirewall_SidecarSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "firewall-rules.json")
	b := newFake()
	require.NoError(t, newLinux(path, b).AddNamed(Rule{Name: RuleDNSUDP, Protocol: "UDP", Ports: []int{53, 853}}))
	require.True(t, b.open["home 53/udp"])
	require.NoError(t, newLinux(path, b).DeleteNamed(RuleDNSUDP))
	require.Empty(t, b.open)
	require.NoError(t, newLinux(path, b).DeleteNamed(RuleDNSUDP), "unknown rule: nothing to do")
}

func TestLinuxFirewall_ProxyRuleReplacesPort(t *testing.T) {
	b := newFake()
	m := newLinux(filepath.Join(t.TempDir(), "f.json"), b)
	require.NoError(t, m.Add(8080))
	require.NoError(t, m.Add(9090))
	require.Equal(t, map[string]bool{"home 9090/tcp": true}, b.open)
	require.NoError(t, m.Delete())
	require.Empty(t, b.open)
}

// Review Focus 4: a port the user had opened stays open.
func TestLinuxFirewall_PreexistingPortNotClosed(t *testing.T) {
	b := newFake()
	b.open["home 8080/tcp"] = true
	m := newLinux(filepath.Join(t.TempDir(), "f.json"), b)
	require.NoError(t, m.Add(8080))
	require.NoError(t, m.Delete())
	require.True(t, b.open["home 8080/tcp"])
	for _, l := range b.log {
		require.False(t, strings.HasPrefix(l, "open") || strings.HasPrefix(l, "close"), l)
	}
}

func TestLinuxFirewall_BlockRuleIsNoop(t *testing.T) {
	b := newFake()
	require.NoError(t, newLinux(filepath.Join(t.TempDir(), "f.json"), b).AddNamed(BlockPublicRule))
	require.Empty(t, b.log)
}

func TestLinuxFirewall_Public(t *testing.T) {
	b := newFake()
	b.pub = true
	pub, err := newLinux(filepath.Join(t.TempDir(), "f.json"), b).IsPublicNetwork()
	require.NoError(t, err)
	require.True(t, pub)
}
