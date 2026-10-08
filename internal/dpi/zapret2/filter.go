package zapret2

import (
	"strconv"
	"strings"

	"github.com/hashcott/ghostline/internal/dpi"
)

// Filter is what the engine sees: TCP 80/443 and QUIC (UDP 443). Packet
// counts are those of zapret2's config.default for nfqws2.
var Filter = []dpi.CaptureRule{
	{Proto: "tcp", Ports: []int{80, 443}, OutPackets: 20, InPackets: 10},
	{Proto: "udp", Ports: []int{443}, OutPackets: 5, InPackets: 3, QUIC: true},
}

const (
	// QueueNum is the NFQUEUE nfqws2 reads on Linux.
	QueueNum = 200
	// FWMark marks the packets nfqws2 sends itself, so they are not
	// queued again (nfqws2's default).
	FWMark = 0x40000000
)

func portsCSV(r dpi.CaptureRule) string {
	s := make([]string, len(r.Ports))
	for i, p := range r.Ports {
		s[i] = strconv.Itoa(p)
	}
	return strings.Join(s, ",")
}
