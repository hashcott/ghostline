package dpi

// CaptureRule is one line of an engine's port table: which packets of which
// connections go to the engine. Windows turns it into WinDivert filters,
// Linux into nftables rules, so the two cannot drift apart.
type CaptureRule struct {
	Proto      string // "tcp" | "udp"
	Ports      []int  // remote ports
	OutPackets int    // first packets of the connection's outgoing direction
	InPackets  int    // first reply packets (for auto-detecting blocked sites)
	QUIC       bool   // only needed when the strategy has a QUIC profile
}
