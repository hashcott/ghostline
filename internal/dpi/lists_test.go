package dpi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Review C2: the engine (nobody on Linux) can replace its list with a link
// to any file. Root never writes through it…
func TestCopyLists_DoesNotWriteThroughALink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "saved.txt")
	require.NoError(t, os.WriteFile(src, []byte("a.com\n"), 0o644))
	victim := filepath.Join(t.TempDir(), "sudoers")
	require.NoError(t, os.WriteFile(victim, []byte("root ALL\n"), 0o600))
	dst := filepath.Join(dir, filepath.FromSlash(autoHostlistName))
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0o755))
	require.NoError(t, os.Symlink(victim, dst))
	_, err := copyLists(dir, Plan{Scope: ScopeBlacklist, AutoHostlist: src})
	require.NoError(t, err)
	b, _ := os.ReadFile(victim)
	require.Equal(t, "root ALL\n", string(b), "the link's target is untouched")
	fi, err := os.Lstat(dst)
	require.NoError(t, err)
	require.True(t, fi.Mode().IsRegular(), "a fresh regular file replaced the link")
}

// …and never reads through it (or from a pipe, or without a size limit).
func TestReadEngineList_RejectsLinksAndPipes(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "shadow")
	require.NoError(t, os.WriteFile(victim, []byte("secret"), 0o600))
	link := filepath.Join(dir, "list")
	require.NoError(t, os.Symlink(victim, link))
	_, err := readEngineList(link)
	require.Error(t, err)

	ok := filepath.Join(dir, "ok")
	require.NoError(t, os.WriteFile(ok, make([]byte, maxEngineList+10), 0o644))
	b, err := readEngineList(ok)
	require.NoError(t, err)
	require.Len(t, b, maxEngineList, "capped")
}
