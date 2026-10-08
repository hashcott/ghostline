package dpi

import (
	"encoding/binary"
	"fmt"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
	"golang.org/x/sys/unix"
)

const (
	nftTable  = "ghostline"
	queueNum  = 200
	engineFWM = 0x40000000 // nfqws2's --fwmark for the packets it sends

	// Conntrack directions (enum ip_conntrack_dir).
	ctDirOriginal = 0
	ctDirReply    = 1
)

func ourTable() *nftables.Table {
	return &nftables.Table{Family: nftables.TableFamilyINet, Name: nftTable}
}

func tableExistsOn(c *nftables.Conn) (bool, error) {
	ts, err := c.ListTablesOfFamily(nftables.TableFamilyINet)
	if err != nil {
		return false, err
	}
	for _, t := range ts {
		if t.Name == nftTable {
			return true, nil
		}
	}
	return false, nil
}

// tableExists reports whether Ghostline's table is loaded.
func tableExists() (bool, error) {
	c, err := nftables.New()
	if err != nil {
		return false, err
	}
	return tableExistsOn(c)
}

// installTable replaces Ghostline's table with one built from f, in one
// netlink batch: either the whole new table is in place or nothing changed.
func installTable(f []CaptureRule) error {
	c, err := nftables.New()
	if err != nil {
		return err
	}
	exists, err := tableExistsOn(c)
	if err != nil {
		return err
	}
	t := ourTable()
	if exists {
		c.DelTable(t)
	}
	c.AddTable(t)
	post := c.AddChain(&nftables.Chain{Name: "post", Table: t, Type: nftables.ChainTypeFilter,
		Hooknum: nftables.ChainHookPostrouting, Priority: nftables.ChainPriorityRef(99)})
	pre := c.AddChain(&nftables.Chain{Name: "pre", Table: t, Type: nftables.ChainTypeFilter,
		Hooknum: nftables.ChainHookPrerouting, Priority: nftables.ChainPriorityRef(-99)})
	for _, r := range nftPlan(f) {
		ch := post
		if r.Chain == "pre" {
			ch = pre
		}
		ex, err := r.exprs()
		if err != nil {
			return err
		}
		c.AddRule(&nftables.Rule{Table: t, Chain: ch, Exprs: ex})
	}
	return c.Flush()
}

// deleteTable removes Ghostline's table; none loaded is not an error.
func deleteTable() error {
	c, err := nftables.New()
	if err != nil {
		return err
	}
	exists, err := tableExistsOn(c)
	if err != nil || !exists {
		return err
	}
	c.DelTable(ourTable())
	return c.Flush()
}

// Host-order 32-bit value, as nft keeps the packet mark.
func host32(v uint32) []byte { b := make([]byte, 4); binary.NativeEndian.PutUint32(b, v); return b }
func be16(v uint16) []byte   { b := make([]byte, 2); binary.BigEndian.PutUint16(b, v); return b }
func be64(v uint64) []byte   { b := make([]byte, 8); binary.BigEndian.PutUint64(b, v); return b }

func cmpEq(data []byte) expr.Any { return &expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: data} }

func nfproto(fam string) []expr.Any {
	p := byte(unix.NFPROTO_IPV4)
	if fam == "ip6" {
		p = unix.NFPROTO_IPV6
	}
	return []expr.Any{&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1}, cmpEq([]byte{p})}
}

func l4proto(p byte) []expr.Any {
	return []expr.Any{&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1}, cmpEq([]byte{p})}
}

// markSet: meta mark & FWM != 0.
func markSet() []expr.Any {
	return []expr.Any{
		&expr.Meta{Key: expr.MetaKeyMARK, Register: 1},
		&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4, Mask: host32(engineFWM), Xor: host32(0)},
		&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: host32(0)},
	}
}

func verdict(k expr.VerdictKind) expr.Any { return &expr.Verdict{Kind: k} }

func (r nftRule) exprs() ([]expr.Any, error) {
	var ex []expr.Any
	switch r.Action {
	case "markaccept":
		// … ct mark set ct mark | FWM accept
		ex = append(markSet(),
			&expr.Ct{Register: 1, Key: expr.CtKeyMARK},
			&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4, Mask: host32(^uint32(engineFWM)), Xor: host32(engineFWM)},
			&expr.Ct{Register: 1, SourceRegister: true, Key: expr.CtKeyMARK},
			verdict(expr.VerdictAccept))
	case "loaccept":
		name := make([]byte, unix.IFNAMSIZ)
		copy(name, "lo")
		ex = []expr.Any{&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1}, cmpEq(name), verdict(expr.VerdictAccept)}
	case "netaccept":
		addr := r.Net.Addr().AsSlice()
		off, n := uint32(16), uint32(4) // IPv4 destination
		if r.Net.Addr().Is6() {
			off, n = 24, 16
		}
		mask := make([]byte, n)
		for i := 0; i < r.Net.Bits(); i++ {
			mask[i/8] |= 0x80 >> (i % 8)
		}
		ex = append(nfproto(r.Family),
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseNetworkHeader, Offset: off, Len: n},
			&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: n, Mask: mask, Xor: make([]byte, n)},
			cmpEq(addr),
			verdict(expr.VerdictAccept))
	case "queue":
		proto, portOff := byte(unix.IPPROTO_TCP), uint32(2) // dport
		if r.Proto == "udp" {
			proto = unix.IPPROTO_UDP
		}
		dir := uint32(ctDirOriginal)
		if r.Dir == "reply" {
			portOff, dir = 0, ctDirReply // sport of the server's reply
		}
		ex = append(l4proto(proto),
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: portOff, Len: 2},
			cmpEq(be16(uint16(r.Port))),
			// ct <dir> packets 1-N: the counter is host order; nft compares it
			// in network order after a byteorder conversion.
			&expr.Ct{Register: 1, Key: expr.CtKeyPKTS, Direction: dir},
			&expr.Byteorder{SourceRegister: 1, DestRegister: 1, Op: expr.ByteorderHton, Len: 8, Size: 8},
			&expr.Range{Op: expr.CmpOpEq, Register: 1, FromData: be64(1), ToData: be64(uint64(r.Packets))},
			&expr.Queue{Num: queueNum, Flag: expr.QueueFlagBypass})
	case "ttldrop":
		proto, typ := byte(unix.IPPROTO_ICMP), byte(11) // time-exceeded
		if r.Family == "ip6" {
			proto, typ = unix.IPPROTO_ICMPV6, 3
		}
		ex = append(nfproto(r.Family), l4proto(proto)...)
		ex = append(ex,
			&expr.Payload{DestRegister: 1, Base: expr.PayloadBaseTransportHeader, Offset: 0, Len: 1},
			cmpEq([]byte{typ}),
			&expr.Ct{Register: 1, Key: expr.CtKeyMARK},
			&expr.Bitwise{SourceRegister: 1, DestRegister: 1, Len: 4, Mask: host32(engineFWM), Xor: host32(0)},
			&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: host32(0)},
			verdict(expr.VerdictDrop))
	default:
		return nil, fmt.Errorf("dpi: unknown nft action %q", r.Action)
	}
	return ex, nil
}
