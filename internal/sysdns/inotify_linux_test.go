package sysdns

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatchFile_ReportsRewritesOfThatFileOnly(t *testing.T) {
	old := fileDebounce
	fileDebounce = 50 * time.Millisecond
	defer func() { fileDebounce = old }()
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	require.NoError(t, os.WriteFile(path, []byte("a"), 0o644))
	hits := make(chan struct{}, 8)
	stop, err := watchFile(path, func() { hits <- struct{}{} })
	require.NoError(t, err)
	defer stop()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "other"), []byte("x"), 0o644))
	select {
	case <-hits:
		t.Fatal("another file must not count")
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, writeAtomic(path, []byte("b"), 0o644)) // rename over it, like dhcpcd
	select {
	case <-hits:
	case <-time.After(2 * time.Second):
		t.Fatal("rewrite not reported")
	}
}
