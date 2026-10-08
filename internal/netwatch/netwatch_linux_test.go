package netwatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestWatch_StartsAndStops(t *testing.T) {
	stop, err := Watch(func() {})
	require.NoError(t, err)
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop did not return")
	}
}

// ENOBUFS means the kernel dropped events: something changed that was
// not seen, so it counts as a change.
func TestRecvTriggers(t *testing.T) {
	require.True(t, recvTriggers(40, nil))
	require.True(t, recvTriggers(-1, unix.ENOBUFS))
	require.False(t, recvTriggers(-1, unix.EAGAIN))
	require.False(t, recvTriggers(-1, unix.EINTR))
	require.False(t, recvTriggers(0, nil))
}
