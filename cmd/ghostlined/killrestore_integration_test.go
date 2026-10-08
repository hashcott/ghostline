//go:build linux && integration_root

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

// What systemd's ExecStopPost=--restore does after a kill -9: DNS was
// pointed at Ghostline, its process is gone, and --restore puts it back.
func TestRoot_RestoreAfterKill(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	dir := t.TempDir()
	bin := os.Getenv("GHOSTLINED_BIN") // prebuilt, so root need not run go build in a user's cache
	if bin == "" {
		bin = filepath.Join(dir, "ghostlined")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := build.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	data := filepath.Join(dir, "data")
	require.NoError(t, os.MkdirAll(data, 0o700))
	b := sysdns.DetectLinux(data)
	snap, err := b.Snapshot(sysdns.Selection{Mode: "auto"})
	require.NoError(t, err)
	state := store.CleanState()
	state.Phase, state.PID, state.PIDStartTime, state.DNS = store.PhaseDNSSet, 999999999, time.Unix(1, 0), snap
	require.NoError(t, store.WriteJSONAtomic(filepath.Join(data, "state.json"), state))
	require.NoError(t, b.Apply(snap, false))
	defer b.Restore(snap) // never leave the machine on loopback, even if the test fails
	require.False(t, b.StillOurs(snap).Empty())

	cmd := exec.Command(bin, "--restore", "--data-dir", dir)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	got, err := store.NewStateStore(filepath.Join(data, "state.json"), nopLock{}).Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseClean, got.Phase)
	require.True(t, b.StillOurs(snap).Empty(), "DNS is back as before")
}

type nopLock struct{}

func (nopLock) Lock() error   { return nil }
func (nopLock) Unlock() error { return nil }
