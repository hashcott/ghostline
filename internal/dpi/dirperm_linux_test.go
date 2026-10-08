package dpi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The engine dir is reachable by the drop user only through its group, and
// only auto/ is writable by it (it must never be able to swap nfqws2).
func TestPrepareEngineDir_Modes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ghostline")
	dir := filepath.Join(root, "bin", "zapret2")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "lua"), 0o700))
	require.NoError(t, os.Chmod(root, 0o700))
	for _, f := range []string{"nfqws2", "lua/zapret-lib.lua", "blacklist.txt"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600))
	}
	uid, gid := os.Getuid(), os.Getgid() // chown to oneself works without root
	require.NoError(t, prepareEngineDirAs(dir, "nfqws2", uid, gid))
	mode := func(p string) os.FileMode { fi, err := os.Stat(p); require.NoError(t, err); return fi.Mode().Perm() }
	require.Equal(t, os.FileMode(0o750), mode(dir))
	require.Equal(t, os.FileMode(0o750), mode(filepath.Join(dir, "lua")))
	require.Equal(t, os.FileMode(0o755), mode(filepath.Join(dir, "nfqws2")))
	require.Equal(t, os.FileMode(0o644), mode(filepath.Join(dir, "lua/zapret-lib.lua")))
	require.Equal(t, os.FileMode(0o644), mode(filepath.Join(dir, "blacklist.txt")))
	require.Equal(t, os.FileMode(0o700), mode(filepath.Join(dir, "auto")))
	require.Equal(t, os.FileMode(0o755), mode(filepath.Join(root, "bin")))
	require.NotZero(t, mode(root)&0o001, "the data root is traversable")
	require.Zero(t, mode(root)&0o004, "but not listable")
}
