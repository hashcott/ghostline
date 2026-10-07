package core

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

const newerList = `{"version":2,"zapret2":[{"id":"r","name":{"vi":"v","en":"e"},"tcp":["--lua-desync=pass"],"quic":[]}]}`

func strategyPaths(t *testing.T) store.Paths {
	dir := t.TempDir()
	return store.ResolvePaths(filepath.Join(dir, "ghostline.exe"), dir)
}

func writeRemote(t *testing.T, p store.Paths, raw, sig []byte) {
	require.NoError(t, os.MkdirAll(filepath.Dir(p.DPIStrategies), 0o755))
	require.NoError(t, os.WriteFile(p.DPIStrategies, raw, 0o644))
	require.NoError(t, os.WriteFile(p.DPIStrategiesSig, sig, 0o644))
}

func TestStrategyBox_BuiltinWithoutRemote(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	b := newStrategyBox(strategyPaths(t), pub, slog.New(slog.DiscardHandler))
	require.Equal(t, "z-split", b.get().Zapret2[0].ID)
}

func TestStrategyBox_IgnoresTamperedRemote(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	p := strategyPaths(t)
	writeRemote(t, p, []byte(newerList), servers.Sign([]byte(newerList), other))
	var logs bytes.Buffer
	b := newStrategyBox(p, pub, slog.New(slog.NewTextHandler(&logs, nil)))
	require.Equal(t, "z-split", b.get().Zapret2[0].ID)
	require.Contains(t, logs.String(), "STRATEGY_LIST_INVALID")
}

func TestStrategyBox_UsesNewerSignedRemoteAfterReload(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	p := strategyPaths(t)
	b := newStrategyBox(p, pub, slog.New(slog.DiscardHandler))
	require.Equal(t, "z-split", b.get().Zapret2[0].ID)
	writeRemote(t, p, []byte(newerList), servers.Sign([]byte(newerList), priv))
	b.reload()
	require.Equal(t, "r", b.get().Zapret2[0].ID)
}
