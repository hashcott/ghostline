//go:build linux && integration_root

package sysdns

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These change the machine's real DNS and put it back. Run as root:
//
//	sudo -E env "PATH=$PATH" go test -tags integration_root ./internal/sysdns/ ./cmd/ghostlined/

func needRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
}

func TestRoot_DetectedBackendRoundTrip(t *testing.T) {
	needRoot(t)
	b := DetectLinux(t.TempDir())
	t.Logf("backend: %s (%s)", b.Name(), b.Info().Chain)
	s, err := b.Snapshot(Selection{Mode: "auto"})
	require.NoError(t, err)
	require.True(t, b.StillOurs(s).Empty(), "nothing of Ghostline's before Apply")
	require.NoError(t, b.Apply(s, false))
	defer b.Restore(s) // never leave the machine on loopback
	require.False(t, b.StillOurs(s).Empty(), "Ghostline's configuration is in place")
	if b.Name() == "resolved" {
		_, err := os.Stat(filepath.Join(resolvedDropInDir, "ghostline.conf"))
		require.NoError(t, err)
	}
	require.Empty(t, b.Restore(s))
	require.True(t, b.StillOurs(s).Empty(), "back as before")
	after, err := b.Snapshot(Selection{Mode: "auto"})
	require.NoError(t, err)
	if s.Linux != nil && s.Linux.NMGlobal != nil {
		require.Equal(t, len(s.Linux.NMGlobal.Domains), len(after.Linux.NMGlobal.Domains))
	}
	if s.Linux != nil && len(s.Linux.ResolvedLinks) > 0 {
		require.Equal(t, s.Linux.ResolvedLinks, after.Linux.ResolvedLinks)
	}
}

// The real inotify watch sees an external rewrite of a resolv.conf.
func TestRoot_ResolvConfWatchesRealFile(t *testing.T) {
	needRoot(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	require.NoError(t, os.WriteFile(path, []byte("nameserver 192.0.2.1\n"), 0o644))
	b := newResolvConf(path, dir, func() error { return nil }, func(f func()) (func(), error) { return watchFile(path, f) })
	s, err := b.Snapshot(Selection{})
	require.NoError(t, err)
	require.NoError(t, b.Apply(s, false))
	hit := make(chan struct{}, 4)
	stop, err := b.Watch(func() { hit <- struct{}{} })
	require.NoError(t, err)
	defer stop()
	require.NoError(t, os.WriteFile(path, []byte("nameserver 192.0.2.2\n"), 0o644))
	select {
	case <-hit:
	case <-time.After(3 * time.Second):
		t.Fatal("the rewrite was not seen within 3 s")
	}
	_, toApply, changes, err := b.Reconcile(s, Selection{})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.NoError(t, b.Apply(toApply, false))
	require.False(t, b.StillOurs(s).Empty())
}
