package dpi

import (
	"bufio"
	"bytes"
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

// queueBound reports whether queue num has a reader, from the contents of
// /proc/net/netfilter/nfnetlink_queue (queue number, then peer portid).
func queueBound(proc []byte, num int) bool {
	sc := bufio.NewScanner(bytes.NewReader(proc))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 || f[0] != strconv.Itoa(num) {
			continue
		}
		return f[1] != "0"
	}
	return false
}
