package procs

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// parseProcNet returns the socket inodes bound to local port in a
// /proc/net/{tcp,tcp6,udp,udp6} table. TCP sockets count only while
// listening (state 0A). Sockets on a loopback address other than
// 127.0.0.1 and ::1 (resolved's stub on 127.0.0.53, dnsmasq on 127.0.1.1)
// are left out: they never conflict with Ghostline's.
func parseProcNet(data []byte, port uint16, tcp bool) []uint64 {
	var out []uint64
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 10 {
			continue
		}
		hexAddr, hexPort, ok := strings.Cut(f[1], ":")
		if !ok {
			continue
		}
		p, err := strconv.ParseUint(hexPort, 16, 16)
		if err != nil || uint16(p) != port {
			continue
		}
		if tcp && f[3] != "0A" {
			continue
		}
		if a, ok := procNetAddr(hexAddr); !ok || (a.IsLoopback() && a != netip.MustParseAddr("127.0.0.1") && a != netip.IPv6Loopback()) {
			continue
		}
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		out = append(out, inode)
	}
	return out
}

// procNetAddr decodes a /proc/net address: the bytes of each 32-bit word
// are in host (little-endian) order. A v4-mapped v6 address is unmapped.
func procNetAddr(h string) (netip.Addr, bool) {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return netip.Addr{}, false
	}
	for i := 0; i < len(b); i += 4 {
		slices.Reverse(b[i : i+4])
	}
	a, ok := netip.AddrFromSlice(b)
	return a.Unmap(), ok
}

// unitFromCgroup is the system service a process runs in, from the
// contents of /proc/<pid>/cgroup; "" outside system.slice (a user's own
// units may carry any name).
func unitFromCgroup(cgroup string) string {
	for _, line := range strings.Split(cgroup, "\n") {
		_, path, ok := strings.Cut(line, "::")
		if !ok || !strings.HasPrefix(path, "/system.slice/") {
			continue
		}
		segs := strings.Split(path, "/")
		for i := len(segs) - 1; i >= 0; i-- {
			if strings.HasSuffix(segs[i], ".service") {
				return segs[i]
			}
		}
	}
	return ""
}

var unitName = regexp.MustCompile(`^[A-Za-z0-9:_.@\\-]+\.service$`)

// protectedUnits are never stopped for port 53: DNS itself or Ghostline.
var protectedUnits = []string{"systemd-resolved.service", "NetworkManager.service", "ghostline.service"}

// stoppable reports whether unit may be stopped to free port 53.
func stoppable(unit string) error {
	if !unitName.MatchString(unit) {
		return fmt.Errorf("procs: %q is not a service name", unit)
	}
	if slices.Contains(protectedUnits, unit) {
		return fmt.Errorf("procs: %s is not stopped for port 53", unit)
	}
	return nil
}
