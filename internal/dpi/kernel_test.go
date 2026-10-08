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
	require.True(t, queueBound(bound, 200, 65151))
	require.False(t, queueBound(bound, 200, 4242), "another process reads the queue")
	require.False(t, queueBound(bound, 201, 65151))
	require.False(t, queueBound([]byte("  200      0     0 2 65531     0     0        2  1\n"), 200, 0), "no reader")
	require.False(t, queueBound(nil, 200, 65151))
}

// modules.builtin lists modules compiled into the kernel: they never show
// up in /sys/module unless they have parameters.
func TestBuiltinModules(t *testing.T) {
	got := builtinModules([]byte("kernel/net/netfilter/nft_queue.ko\nkernel/net/netfilter/nfnetlink-queue.ko\n\nkernel/fs/ext4/ext4.ko\n"))
	require.True(t, got["nft_queue"])
	require.True(t, got["nfnetlink_queue"], "names use _ for -")
	require.True(t, got["ext4"])
	require.False(t, got["nf_conntrack"])
	require.Empty(t, builtinModules(nil))
}
