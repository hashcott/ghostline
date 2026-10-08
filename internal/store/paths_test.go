package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPathsIn_MatchesResolvePathsLayout(t *testing.T) {
	dir := t.TempDir()
	want := ResolvePaths(filepath.Join(dir, "ghostline"), dir)
	got := PathsIn(want.DataDir, filepath.Join(dir, "logs-elsewhere"))
	require.Equal(t, filepath.Join(dir, "logs-elsewhere"), got.LogDir)
	require.Equal(t, want.DataDir, got.MachineDir, "the LAN CA stays in the root-only data directory")
	got.LogDir, got.MachineDir = want.LogDir, want.MachineDir
	require.Equal(t, want, got)
}
