package netid

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

// isOnLAN keeps interfaces backed by a network card; container and VM
// bridges (docker0, virbr0, veth…) have none.
func isOnLAN(i net.Interface) bool { return onLAN("/sys/class/net", i.Name) }

// onLAN reports whether name, under sys (/sys/class/net), is a network
// card or sits on one: a bridge, bond or VLAN whose lower_* ports include
// a card. An interface sysfs does not list is kept.
func onLAN(sys, name string) bool {
	if _, err := os.Stat(filepath.Join(sys, name)); err != nil {
		return true
	}
	return backed(sys, name, 0)
}

func backed(sys, name string, depth int) bool {
	if _, err := os.Lstat(filepath.Join(sys, name, "device")); err == nil {
		return true
	}
	if depth >= 8 {
		return false
	}
	lower, _ := filepath.Glob(filepath.Join(sys, name, "lower_*"))
	for _, l := range lower {
		if backed(sys, strings.TrimPrefix(filepath.Base(l), "lower_"), depth+1) {
			return true
		}
	}
	return false
}
