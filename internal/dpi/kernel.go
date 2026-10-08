package dpi

import (
	"bufio"
	"bytes"
	"path"
	"strconv"
	"strings"
)

// kernelModules are what an nftables queue to nfqws2 needs.
var kernelModules = []string{"nfnetlink_queue", "nft_queue", "nf_conntrack"}

// missingModules lists the kernelModules loaded reports absent.
func missingModules(loaded func(name string) bool) []string {
	var out []string
	for _, m := range kernelModules {
		if !loaded(m) {
			out = append(out, m)
		}
	}
	return out
}

// builtinModules reads modules.builtin (one path per line, such as
// kernel/net/netfilter/nft_queue.ko): modules compiled into the kernel,
// which /sys/module lists only when they have parameters.
func builtinModules(list []byte) map[string]bool {
	out := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(list))
	for sc.Scan() {
		if name := strings.TrimSuffix(path.Base(strings.TrimSpace(sc.Text())), ".ko"); name != "." && name != "" {
			out[strings.ReplaceAll(name, "-", "_")] = true
		}
	}
	return out
}

// queueBound reports whether pid reads queue num, from the contents of
// /proc/net/netfilter/nfnetlink_queue (queue number, then peer portid,
// which is the pid of the process that bound it).
func queueBound(proc []byte, num, pid int) bool {
	sc := bufio.NewScanner(bytes.NewReader(proc))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 || f[0] != strconv.Itoa(num) {
			continue
		}
		return f[1] != "0" && f[1] == strconv.Itoa(pid)
	}
	return false
}
