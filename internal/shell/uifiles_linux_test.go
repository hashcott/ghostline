package shell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/stretchr/testify/require"
)

func rawArgs(t *testing.T, args ...any) []json.RawMessage {
	t.Helper()
	out := make([]json.RawMessage, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		require.NoError(t, err)
		out[i] = b
	}
	return out
}

func TestSaveFileHandler_WritesAsCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	var asked string
	h := saveFileHandler(func(name string) (string, error) { asked = name; return path, nil })
	_, err := h(rawArgs(t, "x.ghostline.json", []byte("hello")))
	require.NoError(t, err)
	require.Equal(t, "x.ghostline.json", asked)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))
	fi, err := os.Stat(path)
	require.NoError(t, err)
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	require.Equal(t, os.FileMode(0o644)&^os.FileMode(mask), fi.Mode().Perm())
}

func TestSaveFileHandler_CancelledWritesNothing(t *testing.T) {
	dir := t.TempDir()
	h := saveFileHandler(func(string) (string, error) { return "", nil })
	_, err := h(rawArgs(t, "x.json", []byte("hello")))
	require.NoError(t, err)
	entries, _ := os.ReadDir(dir)
	require.Empty(t, entries)
}

// Review Focus 4: a file over the backup limit is refused before it is sent.
func TestOpenFileHandler_RejectsOversize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.json")
	require.NoError(t, os.WriteFile(path, make([]byte, backup.MaxSize+1), 0o644))
	h := openFileHandler(func(string) (string, error) { return path, nil })
	_, err := h(rawArgs(t, "Ghostline"))
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "IMPORT_INVALID"), err.Error())
}

func TestOpenFileHandler_ReturnsNameAndData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "in.ghostline.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a":1}`), 0o644))
	h := openFileHandler(func(title string) (string, error) { return path, nil })
	res, err := h(rawArgs(t, "Ghostline"))
	require.NoError(t, err)
	b, err := json.Marshal(res)
	require.NoError(t, err)
	var got struct {
		Name string `json:"name"`
		Data []byte `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, "in.ghostline.json", got.Name)
	require.Equal(t, `{"a":1}`, string(got.Data))
}
