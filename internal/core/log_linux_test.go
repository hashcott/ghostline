package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

// The daemon's data directory holds the secret key and the LAN CA: root only.
func TestOpenLog_DataDirIs0700(t *testing.T) {
	dir := t.TempDir()
	paths := store.PathsIn(filepath.Join(dir, "data"), filepath.Join(dir, "log"))
	require.NoError(t, os.MkdirAll(paths.DataDir, 0o755))
	require.NoError(t, os.Chmod(paths.DataDir, 0o755))
	_, closer, err := OpenLog(paths)
	require.NoError(t, err)
	defer closer.Close()
	fi, err := os.Stat(paths.DataDir)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
}
