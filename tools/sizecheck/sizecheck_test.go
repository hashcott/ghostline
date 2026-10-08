package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrew(t *testing.T) {
	pct, ok := Grew(100_000_000, 101_500_000, 2)
	require.InDelta(t, 1.5, pct, 1e-9)
	require.True(t, ok)
	_, ok = Grew(100_000_000, 102_500_000, 2)
	require.False(t, ok)
	_, ok = Grew(100_000_000, 90_000_000, 2) // shrinking is always fine
	require.True(t, ok)
}

func TestEntrySize_ReadsUncompressedSize(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.zip")
	f, err := os.Create(p)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("ghostline.exe")
	require.NoError(t, err)
	_, err = w.Write(make([]byte, 12345))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	n, err := EntrySize(p, "ghostline.exe")
	require.NoError(t, err)
	require.Equal(t, int64(12345), n)
	_, err = EntrySize(p, "missing.exe")
	require.Error(t, err)
}
