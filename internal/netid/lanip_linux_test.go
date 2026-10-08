package netid

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSysNet lays out /sys/class/net: a device link for hardware, lower_*
// links for the ports of a bridge, bond or VLAN.
func fakeSysNet(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mk := func(name string, device bool, lower ...string) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o755))
		if device {
			require.NoError(t, os.Symlink("../../devices/pci0000:00/x", filepath.Join(dir, name, "device")))
		}
		for _, l := range lower {
			require.NoError(t, os.Symlink("../"+l, filepath.Join(dir, name, "lower_"+l)))
		}
	}
	mk("eno1", true)
	mk("wlan0", true)
	mk("docker0", false, "veth1a2b")
	mk("veth1a2b", false)
	mk("virbr0", false)
	mk("br0", false, "eno1") // a host bridge over the real NIC
	mk("eno1.10", false, "eno1")
	mk("bond0", false, "veth1a2b", "wlan0")
	return dir
}

func TestOnLAN(t *testing.T) {
	sys := fakeSysNet(t)
	for name, want := range map[string]bool{
		"eno1": true, "wlan0": true, "br0": true, "eno1.10": true, "bond0": true,
		"docker0": false, "virbr0": false, "veth1a2b": false,
		"unknown0": true, // not in sysfs (a container without it): keep
	} {
		require.Equal(t, want, onLAN(sys, name), name)
	}
}
