package firewall

import (
	"slices"
	"strconv"
)

// firewalldAPI is firewalld's D-Bus interface (firewalld_linux.go).
type firewalldAPI interface {
	QueryPort(zone, port, proto string) (bool, error)
	AddPort(zone, port, proto string) error    // runtime only: gone at reload/reboot
	RemovePort(zone, port, proto string) error // not open is not an error
	ZoneOfInterface(iface string) (string, error)
	DefaultZone() (string, error)
}

// firewalld opens ports in the zone of the LAN interface.
type firewalld struct {
	api   firewalldAPI
	iface func() string // the default route's interface
}

func (f firewalld) zone() (string, error) {
	z, err := f.api.ZoneOfInterface(f.iface())
	if err != nil || z == "" {
		return f.api.DefaultZone()
	}
	return z, nil
}

func (f firewalld) exists(zone, proto string, port int) (bool, error) {
	return f.api.QueryPort(zone, strconv.Itoa(port), proto)
}

func (f firewalld) add(zone, proto string, port int, _ string) error {
	return f.api.AddPort(zone, strconv.Itoa(port), proto)
}

func (f firewalld) remove(zone, proto string, port int) error {
	return f.api.RemovePort(zone, strconv.Itoa(port), proto)
}

// public: zones meant for untrusted networks keep LAN devices out.
func (f firewalld) public() (bool, error) {
	z, err := f.zone()
	if err != nil {
		return false, err
	}
	return slices.Contains([]string{"public", "external", "block", "drop"}, z), nil
}
