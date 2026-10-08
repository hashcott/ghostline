package certstore

import (
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnchors_InstallListRemove(t *testing.T) {
	dir := t.TempDir()
	var ran []string
	a := NewAnchors(dir, "update-ca-trust", func(name string, args ...string) ([]byte, error) { ran = append(ran, name); return nil, nil })
	require.Equal(t, "system", a.Name())
	der := testCA(t, "Ghostline Fake SNI 7")
	require.NoError(t, a.Install(der))
	path := filepath.Join(dir, "ghostline-"+Thumbprint(der)+".crt")
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	blk, _ := pem.Decode(b)
	require.Equal(t, der, blk.Bytes)
	fi, _ := os.Stat(path)
	require.Equal(t, os.FileMode(0o644), fi.Mode().Perm())
	require.Equal(t, []string{"update-ca-trust"}, ran)

	l, err := a.List("Ghostline Fake SNI")
	require.NoError(t, err)
	require.Len(t, l, 1)
	require.Equal(t, Thumbprint(der), l[0].Thumbprint)
	require.Equal(t, "Ghostline Fake SNI 7", l[0].Subject)

	require.NoError(t, a.Remove(Thumbprint(der)))
	require.NoFileExists(t, path)
	require.Equal(t, []string{"update-ca-trust", "update-ca-trust"}, ran)
	require.NoError(t, a.Remove(Thumbprint(der)), "missing is not an error")
	require.Len(t, ran, 2, "nothing removed: no store rebuild")
}

// Only Ghostline's own files count; an admin's CA in the same dir is never
// listed or touched.
func TestAnchors_IgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	other := testCA(t, "Ghostline Fake SNI but not ours")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "corp-root.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other}), 0o644))
	a := NewAnchors(dir, "update-ca-certificates", func(string, ...string) ([]byte, error) { return nil, nil })
	l, err := a.List("Ghostline Fake SNI")
	require.NoError(t, err)
	require.Empty(t, l)
}

func TestAnchorsFor(t *testing.T) {
	has := func(tools ...string) func(string) bool {
		return func(n string) bool {
			for _, x := range tools {
				if x == n {
					return true
				}
			}
			return false
		}
	}
	dir, tool, ok := anchorsFor(has("update-ca-certificates"), func(string) bool { return false })
	require.True(t, ok)
	require.Equal(t, "/usr/local/share/ca-certificates", dir)
	require.Equal(t, "update-ca-certificates", tool)
	dir, tool, _ = anchorsFor(has("update-ca-trust"), func(p string) bool { return p == "/etc/pki/ca-trust/source/anchors" })
	require.Equal(t, "/etc/pki/ca-trust/source/anchors", dir)
	require.Equal(t, "update-ca-trust", tool)
	dir, _, _ = anchorsFor(has("update-ca-trust"), func(string) bool { return false })
	require.Equal(t, "/etc/ca-certificates/trust-source/anchors", dir)
	_, _, ok = anchorsFor(has(), func(string) bool { return false })
	require.False(t, ok)
}
