package procs

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// parseProcNet returns the socket inodes bound to local port in a
// /proc/net/{tcp,tcp6,udp,udp6} table. TCP sockets count only while
// listening (state 0A).
func parseProcNet(data []byte, port uint16, tcp bool) []uint64 {
	var out []uint64
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Scan() // header
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 10 {
			continue
		}
		_, hexPort, ok := strings.Cut(f[1], ":")
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
		inode, err := strconv.ParseUint(f[9], 10, 64)
		if err != nil || inode == 0 {
			continue
		}
		out = append(out, inode)
	}
	return out
}
