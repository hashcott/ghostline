package dpi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingModules(t *testing.T) {
	loaded := map[string]bool{"nfnetlink_queue": true, "nf_conntrack": true}
	require.Equal(t, []string{"nft_queue"}, missingModules(func(m string) bool { return loaded[m] }))
	require.Empty(t, missingModules(func(string) bool { return true }))
}

// /proc/net/netfilter/nfnetlink_queue: queue, peer portid, waiting, copy
// mode, copy range, dropped, user dropped, last id, 1.
func TestQueueBound(t *testing.T) {
	bound := []byte("  200  65151     0 2 65531     0     0        2  1\n")
	require.True(t, queueBound(bound, 200))
	require.False(t, queueBound(bound, 201))
	require.False(t, queueBound([]byte("  200      0     0 2 65531     0     0        2  1\n"), 200), "no reader")
	require.False(t, queueBound(nil, 200))
}
