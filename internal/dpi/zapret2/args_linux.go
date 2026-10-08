package zapret2

import (
	"fmt"

	"github.com/hashcott/ghostline/internal/dpi"
)

const exeName = "nfqws2"

// Pinned is what the Linux build extracts and verifies.
var Pinned = PinsFor(LinuxPins)

// interceptArgs tell nfqws2 which queue to read, how its own packets are
// marked, and whom to drop to; nftables decides what is queued (Filter).
func interceptArgs(bool) []string {
	return []string{fmt.Sprintf("--qnum=%d", dpi.QueueNum), fmt.Sprintf("--fwmark=0x%x", dpi.FWMark), "--user=" + dpi.DropUser}
}
