package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type xorProtector struct{}

func (xorProtector) Protect(b []byte) ([]byte, error) {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[i] ^ 0x33
	}
	return out, nil
}
func (p xorProtector) Unprotect(b []byte) ([]byte, error) { return p.Protect(b) }

func testCertWiring(t *testing.T, st *certstore.Fake, dir string) *certWiring {
	p := store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), dir)
	require.NoError(t, os.MkdirAll(p.DataDir, 0o755))
	return &certWiring{paths: p, store: st, prot: xorProtector{}, host: func() string { return "PC" }, now: time.Now}
}

func TestCertWiring_LANCAPersistsAndIsInstalled(t *testing.T) {
	st := certstore.NewFake()
	dir := t.TempDir()
	a := testCertWiring(t, st, dir)
	ca, err := a.LANCA(context.Background())
	require.NoError(t, err)
	require.True(t, st.Has(ca.Thumbprint()))

	// A new process loads the same CA from disk.
	b := testCertWiring(t, st, dir)
	ca2, err := b.LANCA(context.Background())
	require.NoError(t, err)
	require.Equal(t, ca.Thumbprint(), ca2.Thumbprint())

	// Removed from Root by hand: installed again.
	require.NoError(t, st.Remove(ca.Thumbprint()))
	_, err = b.LANCA(context.Background())
	require.NoError(t, err)
	require.True(t, st.Has(ca.Thumbprint()))
}

func TestCertWiring_ResetAndRemove(t *testing.T) {
	st := certstore.NewFake()
	w := testCertWiring(t, st, t.TempDir())
	old, err := w.LANCA(context.Background())
	require.NoError(t, err)
	fresh, err := w.ResetLANCA(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, old.Thumbprint(), fresh.Thumbprint())
	require.False(t, st.Has(old.Thumbprint()))
	require.True(t, st.Has(fresh.Thumbprint()))

	require.NoError(t, w.RemoveLANCA(context.Background()))
	require.False(t, st.Has(fresh.Thumbprint()))
	_, err = os.Stat(w.paths.LANCAKey)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCertWiring_Sessions(t *testing.T) {
	st := certstore.NewFake()
	w := testCertWiring(t, st, t.TempDir())
	ca, err := certs.NewSessionCA([]string{"example.com"}, time.Now())
	require.NoError(t, err)
	require.NoError(t, w.InstallSession(ca.DER))
	list, err := w.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, w.RemoveSession(ca.Thumbprint()))
	require.False(t, st.Has(ca.Thumbprint()))
}

func TestMITMBox(t *testing.T) {
	var b mitmBox
	require.Nil(t, b.get())
	ca, err := certs.NewSessionCA([]string{"example.com"}, time.Now())
	require.NoError(t, err)
	is := certs.NewIssuer(ca, time.Now)
	b.set(is)
	require.Same(t, is, b.get())
	b.set(nil)
	require.Nil(t, b.get())
}

// A LAN CA whose files are not owned by Administrators/SYSTEM (planted by
// a normal user) is replaced by a fresh one, never installed.
func TestCertWiring_ForeignOwnedFilesReplaced(t *testing.T) {
	st := certstore.NewFake()
	dir := t.TempDir()
	planted := testCertWiring(t, st, dir)
	evil, err := planted.LANCA(context.Background())
	require.NoError(t, err)
	require.NoError(t, st.Remove(evil.Thumbprint()))

	w := testCertWiring(t, st, dir)
	w.owned = func(string) (bool, error) { return false, nil }
	ca, err := w.LANCA(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, evil.Thumbprint(), ca.Thumbprint())
	require.False(t, st.Has(evil.Thumbprint()))
}
