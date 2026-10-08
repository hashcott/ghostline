package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileKey_RoundTrip(t *testing.T) {
	p := NewFileKey(filepath.Join(t.TempDir(), "secret.key"))
	enc, err := EncodeString(p, "pässwörd")
	require.NoError(t, err)
	require.NotContains(t, enc, "pässwörd")
	got, err := DecodeString(p, enc)
	require.NoError(t, err)
	require.Equal(t, "pässwörd", got)
	// A second protector on the same key file reads it back.
	got, err = DecodeString(NewFileKey(filepath.Join(filepath.Dir(p.(*fileKey).path), "secret.key")), enc)
	require.NoError(t, err)
	require.Equal(t, "pässwörd", got)
}

func TestFileKey_KeyFileIs0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	_, err := NewFileKey(path).Protect([]byte("x"))
	require.NoError(t, err)
	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	require.Equal(t, int64(32), fi.Size())
}

func TestFileKey_OtherKeyCannotDecrypt(t *testing.T) {
	a, b := NewFileKey(filepath.Join(t.TempDir(), "a.key")), NewFileKey(filepath.Join(t.TempDir(), "b.key"))
	blob, err := a.Protect([]byte("secret"))
	require.NoError(t, err)
	_, err = b.Unprotect(blob)
	require.Error(t, err)
}

func TestFileKey_TamperedBlobFails(t *testing.T) {
	p := NewFileKey(filepath.Join(t.TempDir(), "secret.key"))
	blob, err := p.Protect([]byte("secret"))
	require.NoError(t, err)
	blob[len(blob)-1] ^= 1
	_, err = p.Unprotect(blob)
	require.Error(t, err)
	_, err = p.Unprotect([]byte("short"))
	require.Error(t, err)
}

// An empty key file is what a crash between creating it and writing it
// left (older builds): nothing was ever encrypted under it, so it is
// replaced instead of failing every start.
func TestFileKey_ReplacesEmptyKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.key")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	_, err := NewFileKey(path).Protect([]byte("x"))
	require.NoError(t, err)
	fi, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, int64(32), fi.Size())
	require.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
	left, _ := filepath.Glob(filepath.Join(dir, ".secret.key-*"))
	require.Empty(t, left, "no temporary file left behind")
}

// A key file someone else wrote is never replaced.
func TestFileKey_KeepsShortNonEmptyKeyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	require.NoError(t, os.WriteFile(path, []byte("short"), 0o600))
	_, err := NewFileKey(path).Protect([]byte("x"))
	require.Error(t, err)
	got, _ := os.ReadFile(path)
	require.Equal(t, "short", string(got))
}
