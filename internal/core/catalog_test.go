package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func countSource(all []model.Server, src model.Source) int {
	n := 0
	for _, s := range all {
		if s.Source == src {
			n++
		}
	}
	return n
}

func TestCatalog_FreshInstallHasTheDNSCryptList(t *testing.T) {
	dir := t.TempDir()
	c := newCatalog(store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), dir))
	require.Greater(t, countSource(c.get(), model.SourceDNSCrypt), 500, "the built-in copy is used before the first download")
}

func TestCatalog_TamperedDownloadFallsBackToBuiltIn(t *testing.T) {
	dir := t.TempDir()
	p := store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), dir)
	require.NoError(t, os.MkdirAll(p.DataDir, 0o755))
	require.NoError(t, os.WriteFile(p.ServersDNSCrypt, []byte("## evil\n\nsdns://AgcAAAAAAAAABzEuMS4xLjEAEmRucy5leGFtcGxlLmNvbQovZG5zLXF1ZXJ5\n"), 0o644))
	require.NoError(t, os.WriteFile(p.ServersDNSCryptSig, []byte("not a signature"), 0o644))
	c := newCatalog(p)
	require.Greater(t, countSource(c.get(), model.SourceDNSCrypt), 500)
	for _, s := range c.get() {
		require.NotEqual(t, "evil", s.Name)
	}
}
