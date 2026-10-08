package sysdns

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Root run: while connected (drop-in DNS=127.0.0.1 ::1), resolved listed a
// server on ifindex 1 (lo); Reconcile took lo for a new link, could not
// turn its DefaultRoute off, and Disconnect then failed on it too.
func TestLinksFromDNS_SkipsLoopback(t *testing.T) {
	rows := [][]any{
		{int32(0), int32(2), []byte{127, 0, 0, 1}},                         // global: Ghostline
		{int32(1), int32(10), []byte{15: 1}},                               // ::1 reported on lo
		{int32(3), int32(2), []byte{1, 0, 0, 1}},                           // wlan0
		{int32(3), int32(10), []byte{0xfd, 0xe5, 0x0c, 0xdf, 0x24, 15: 1}}, // wlan0 v6
		{int32(5), int32(2), []byte{127, 0, 0, 53}},                        // a stub on loopback
	}
	names := map[int]string{1: "lo", 3: "wlan0", 5: "veth0"}
	loop := map[int]bool{1: true}
	got := linksFromDNS(rows, func(i int) (string, bool) { return names[i], loop[i] })
	require.Len(t, got, 1)
	require.Equal(t, 3, got[0].IfIndex)
	require.Equal(t, "wlan0", got[0].Name)
	require.Equal(t, []string{"1.0.0.1", "fde5:cdf:2400::1"}, got[0].Servers)
}
