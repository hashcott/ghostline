package platform

import (
	"path/filepath"
	"sync"
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

// One fileLock shared by goroutines (state.json writers in one process)
// must serialise them without a data race.
func TestFileLock_SharedAcrossGoroutines(t *testing.T) {
	l := newFileLock(filepath.Join(t.TempDir(), "state.lock"))
	var wg sync.WaitGroup
	counter := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				require.NoError(t, l.Lock())
				counter++
				require.NoError(t, l.Unlock())
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 160, counter)
}

// Root run: `ghostlined --restore` after a reboot finds no /run/ghostline
// yet; the lock makes its own directory.
func TestFileLock_CreatesItsDirectory(t *testing.T) {
	l := newFileLock(filepath.Join(t.TempDir(), "run", "state.lock"))
	require.NoError(t, l.Lock())
	require.NoError(t, l.Unlock())
}
