package platform

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileLock_Exclusive(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.lock")
	a, b := newFileLock(p), newFileLock(p)
	require.NoError(t, a.Lock())
	got := make(chan struct{})
	go func() { _ = b.Lock(); close(got) }()
	select {
	case <-got:
		t.Fatal("second lock acquired while the first is held")
	case <-time.After(100 * time.Millisecond):
	}
	require.NoError(t, a.Unlock())
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock never acquired")
	}
	require.NoError(t, b.Unlock())
}
