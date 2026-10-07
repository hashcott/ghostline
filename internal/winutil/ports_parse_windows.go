package winutil

import (
	"encoding/binary"
	"net/netip"
)

type portRow struct {
	addr netip.Addr
	port uint16
	pid  uint32
	tcp  bool
}

// Rows start after the 4-byte dwNumEntries. Ports are stored in network byte
// order in the low two bytes of a DWORD; addresses in memory (network) order.

func parseUDP4Table(b []byte) []portRow {
	return parseTable(b, 12, func(r []byte) portRow {
		return portRow{addr: netip.AddrFrom4([4]byte(r[0:4])), port: binary.BigEndian.Uint16(r[4:6]), pid: binary.LittleEndian.Uint32(r[8:12])}
	})
}

func parseUDP6Table(b []byte) []portRow {
	return parseTable(b, 28, func(r []byte) portRow {
		return portRow{addr: netip.AddrFrom16([16]byte(r[0:16])), port: binary.BigEndian.Uint16(r[20:22]), pid: binary.LittleEndian.Uint32(r[24:28])}
	})
}

// MIB_TCPROW_OWNER_PID{state, localAddr, localPort, remoteAddr, remotePort, pid}
func parseTCP4Table(b []byte) []portRow {
	return parseTable(b, 24, func(r []byte) portRow {
		return portRow{addr: netip.AddrFrom4([4]byte(r[4:8])), port: binary.BigEndian.Uint16(r[8:10]), pid: binary.LittleEndian.Uint32(r[20:24]), tcp: true}
	})
}

// MIB_TCP6ROW_OWNER_PID{localAddr[16], scope, localPort, remoteAddr[16], scope, remotePort, state, pid}
func parseTCP6Table(b []byte) []portRow {
	return parseTable(b, 56, func(r []byte) portRow {
		return portRow{addr: netip.AddrFrom16([16]byte(r[0:16])), port: binary.BigEndian.Uint16(r[20:22]), pid: binary.LittleEndian.Uint32(r[52:56]), tcp: true}
	})
}

func parseTable(b []byte, rowSize int, parse func([]byte) portRow) []portRow {
	if len(b) < 4 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b[0:4]))
	var out []portRow
	for i := 0; i < n; i++ {
		off := 4 + i*rowSize
		if off+rowSize > len(b) {
			break
		}
		out = append(out, parse(b[off:off+rowSize]))
	}
	return out
}
