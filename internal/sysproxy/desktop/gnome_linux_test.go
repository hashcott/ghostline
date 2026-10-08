package desktop

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeGSettings keeps "schema key" → GVariant text, like dconf would.
type fakeGSettings struct {
	vals map[string]string
	log  []string
}

func (f *fakeGSettings) run(name string, args ...string) ([]byte, error) {
	if name == "dconf" && args[0] == "read" { // set keys only, like dconf
		for _, k := range gnomeKeys {
			if dconfPath(k[0], k[1]) == args[1] {
				return []byte(f.vals[k[0]+" "+k[1]] + "\n"), nil
			}
		}
		return nil, fmt.Errorf("unknown path %s", args[1])
	}
	if name != "gsettings" {
		return nil, fmt.Errorf("unexpected %s", name)
	}
	f.log = append(f.log, strings.Join(args, " "))
	k := args[1] + " " + args[2]
	switch args[0] {
	case "get":
		v, ok := f.vals[k]
		if !ok {
			v = gnomeDefaults[k]
		}
		return []byte(v + "\n"), nil
	case "set":
		f.vals[k] = args[3]
	case "reset":
		delete(f.vals, k)
	}
	return nil, nil
}

var gnomeDefaults = map[string]string{
	"org.gnome.system.proxy mode": "'none'", "org.gnome.system.proxy autoconfig-url": "''",
	"org.gnome.system.proxy ignore-hosts": "['localhost', '127.0.0.0/8', '::1']", "org.gnome.system.proxy use-same-proxy": "true",
	"org.gnome.system.proxy.http host": "''", "org.gnome.system.proxy.http port": "8080", "org.gnome.system.proxy.http enabled": "false",
	"org.gnome.system.proxy.https host": "''", "org.gnome.system.proxy.https port": "0",
	"org.gnome.system.proxy.socks host": "''", "org.gnome.system.proxy.socks port": "0",
	"org.gnome.system.proxy.ftp host": "''", "org.gnome.system.proxy.ftp port": "0",
}

func TestGNOME_ApplySetsManualAndHosts(t *testing.T) {
	f := &fakeGSettings{vals: map[string]string{}}
	g := gnome{run: f.run}
	require.NoError(t, g.apply("127.0.0.1:8080"))
	require.Equal(t, "'manual'", f.vals["org.gnome.system.proxy mode"])
	require.Equal(t, "'127.0.0.1'", f.vals["org.gnome.system.proxy.http host"])
	require.Equal(t, "8080", f.vals["org.gnome.system.proxy.http port"])
	require.Equal(t, "'127.0.0.1'", f.vals["org.gnome.system.proxy.https host"])
	require.Equal(t, "['localhost', '127.0.0.0/8', '::1', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']", f.vals["org.gnome.system.proxy ignore-hosts"])
	require.Equal(t, "set org.gnome.system.proxy mode 'manual'", f.log[len(f.log)-1], "the mode flips last, once the hosts are in place")
	ours, err := g.isOurs("127.0.0.1:8080")
	require.NoError(t, err)
	require.True(t, ours)
}

// Every key goes back exactly: set ones to their value, unset ones reset.
func TestGNOME_RoundTripRestoresEveryKey(t *testing.T) {
	f := &fakeGSettings{vals: map[string]string{"org.gnome.system.proxy mode": "'auto'", "org.gnome.system.proxy autoconfig-url": "'http://wpad/x.pac'"}}
	before := map[string]string{}
	for k, v := range f.vals {
		before[k] = v
	}
	g := gnome{run: f.run}
	s, err := g.snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	require.Equal(t, "gnome", s.Backend)
	require.NoError(t, g.apply("127.0.0.1:8080"))
	require.NoError(t, g.restore(s))
	require.Equal(t, before, f.vals)
}

// Ghostline's own leftover (a crash before it was recorded): restore means
// back to the defaults.
func TestGNOME_SnapshotOfOwnLeftoverResets(t *testing.T) {
	f := &fakeGSettings{vals: map[string]string{}}
	g := gnome{run: f.run}
	require.NoError(t, g.apply("127.0.0.1:8080"))
	s, err := g.snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	require.Empty(t, s.GNOME.Values)
	require.NoError(t, g.restore(s))
	require.Empty(t, f.vals)
}

func TestDconfPath(t *testing.T) {
	require.Equal(t, "/system/proxy/mode", dconfPath("org.gnome.system.proxy", "mode"))
	require.Equal(t, "/system/proxy/http/host", dconfPath("org.gnome.system.proxy.http", "host"))
}
