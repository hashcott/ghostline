//go:build linux && integration_root

package dpi

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func needRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
}

func listTable(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("nft", "list", "table", "inet", "ghostline").CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

// The netlink rules read back as the nft rules the L4 spike loaded.
func TestNft_InstallListDelete(t *testing.T) {
	needRoot(t)
	require.NoError(t, installTable(testFilter))
	defer deleteTable()
	got := listTable(t)
	for _, want := range []string{
		"hook postrouting priority srcnat - 1",
		"hook prerouting priority dstnat + 1",
		"meta mark & 0x40000000 != 0x00000000 ct mark set ct mark | 0x40000000 accept",
		`oifname "lo" accept`,
		"ip daddr 192.168.0.0/16 accept",
		"ip6 daddr fc00::/7 accept",
		"tcp dport 443 ct original packets 1-20 queue flags bypass to 200",
		"udp dport 443 ct original packets 1-5 queue flags bypass to 200",
		"tcp sport 80 ct reply packets 1-10 queue flags bypass to 200",
		"icmp type time-exceeded ct mark & 0x40000000 != 0x00000000 drop",
		"icmpv6 type time-exceeded ct mark & 0x40000000 != 0x00000000 drop",
	} {
		require.Contains(t, got, want)
	}
	ok, err := tableExists()
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, deleteTable())
	ok, err = tableExists()
	require.NoError(t, err)
	require.False(t, ok)
}

// Review Focus 1: a daemon restarted while its table is loaded replaces it.
func TestNft_InstallReplacesExistingTable(t *testing.T) {
	needRoot(t)
	require.NoError(t, installTable(testFilter))
	defer deleteTable()
	first := strings.Count(listTable(t), "\n")
	require.NoError(t, installTable(testFilter))
	require.Equal(t, first, strings.Count(listTable(t), "\n"))
}

func TestNft_DeleteMissingIsNil(t *testing.T) {
	needRoot(t)
	require.NoError(t, deleteTable())
	require.NoError(t, deleteTable())
}
