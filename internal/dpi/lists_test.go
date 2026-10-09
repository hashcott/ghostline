package dpi

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// symlinkOrSkip makes a link, or skips where Windows needs admin or Developer
// Mode to make one (CI runners have it; a plain user account does not).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("cannot make a symlink here: %v", err)
		}
		require.NoError(t, err)
	}
}

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
	symlinkOrSkip(t, victim, dst)
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
	symlinkOrSkip(t, victim, link)
	_, err := readEngineList(link)
	require.Error(t, err)

	ok := filepath.Join(dir, "ok")
	require.NoError(t, os.WriteFile(ok, make([]byte, maxEngineList+10), 0o644))
	b, err := readEngineList(ok)
	require.NoError(t, err)
	require.Len(t, b, maxEngineList, "capped")
}
