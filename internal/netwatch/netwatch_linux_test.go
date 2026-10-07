package netwatch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
