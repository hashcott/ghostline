package firewall

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/google/nftables"
)

// DetectLinux picks firewalld, then ufw, then none; opened ports are
// recorded in dataDir/firewall-rules.json.
func DetectLinux(dataDir string) Manager {
	path := filepath.Join(dataDir, "firewall-rules.json")
	if conn, err := dbus.ConnectSystemBus(); err == nil {
		var ok bool
		if conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, fwdName).Store(&ok) == nil && ok {
			return newLinux(path, firewalld{api: dbusFirewalld{conn}, iface: defaultIface})
		}
	}
	run := func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).CombinedOutput() }
	if _, err := exec.LookPath("ufw"); err == nil && ufwActive(run) {
		return newLinux(path, ufw{run: run})
	}
	return newLinux(path, none{inputDrop: inputDropElsewhere})
}

// defaultIface is the default route's interface (/proc/net/route).
func defaultIface() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan() // header
	for sc.Scan() {
		if fs := strings.Fields(sc.Text()); len(fs) > 1 && fs[1] == "00000000" {
			return fs[0]
		}
	}
	return ""
}

// inputDropElsewhere reports an nftables input chain, in a table that is
// not Ghostline's, that drops by default.
func inputDropElsewhere() (bool, error) {
	c, err := nftables.New()
	if err != nil {
		return false, err
	}
	chains, err := c.ListChains()
	if err != nil {
		return false, err
	}
	for _, ch := range chains {
		if ch.Table != nil && ch.Table.Name == "ghostline" {
			continue
		}
		if ch.Hooknum != nil && *ch.Hooknum == *nftables.ChainHookInput && ch.Policy != nil && *ch.Policy == nftables.ChainPolicyDrop {
			return true, nil
		}
	}
	return false, nil
}

const (
	fwdName = "org.fedoraproject.FirewallD1"
	fwdPath = "/org/fedoraproject/FirewallD1"
	fwdZone = fwdName + ".zone"
)

type dbusFirewalld struct{ conn *dbus.Conn }

func (d dbusFirewalld) obj() dbus.BusObject { return d.conn.Object(fwdName, fwdPath) }

func (d dbusFirewalld) QueryPort(zone, port, proto string) (bool, error) {
	var ok bool
	err := d.obj().Call(fwdZone+".queryPort", 0, zone, port, proto).Store(&ok)
	return ok, err
}

func (d dbusFirewalld) AddPort(zone, port, proto string) error {
	return d.obj().Call(fwdZone+".addPort", 0, zone, port, proto, int32(0)).Err
}

func (d dbusFirewalld) RemovePort(zone, port, proto string) error {
	err := d.obj().Call(fwdZone+".removePort", 0, zone, port, proto).Err
	if err != nil && strings.Contains(err.Error(), "NOT_ENABLED") {
		return nil
	}
	return err
}

func (d dbusFirewalld) ZoneOfInterface(iface string) (string, error) {
	var z string
	err := d.obj().Call(fwdZone+".getZoneOfInterface", 0, iface).Store(&z)
	return z, err
}

func (d dbusFirewalld) DefaultZone() (string, error) {
	var z string
	err := d.obj().Call(fwdName+".getDefaultZone", 0).Store(&z)
	return z, err
}
