package zapret2

import "fmt"

const exeName = "nfqws2"

// Pinned is what the Linux build extracts and verifies.
var Pinned = PinsFor(LinuxPins)

// dropUser is who nfqws2 runs as once it holds the queue.
const dropUser = "nobody"

// interceptArgs tell nfqws2 which queue to read, how its own packets are
// marked, and whom to drop to; nftables decides what is queued (Filter).
func interceptArgs(bool) []string {
	return []string{fmt.Sprintf("--qnum=%d", QueueNum), fmt.Sprintf("--fwmark=0x%x", FWMark), "--user=" + dropUser}
}
