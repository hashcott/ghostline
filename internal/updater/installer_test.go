package updater_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/servers"
	"github.com/hashcott/ghostline/internal/updater"
	"github.com/stretchr/testify/require"
)

const asset = "ghostline-amd64-installer.exe"

// release serves an installer, SHA256SUMS listing it (and the portable
// zip of v0.3.0) and the signature of SHA256SUMS.
func release(t *testing.T, priv ed25519.PrivateKey, installer []byte, edit func(sums []byte) []byte) updater.Release {
	sum := sha256.Sum256(installer)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n" +
		"0000000000000000000000000000000000000000000000000000000000000000  Ghostline-0.3.0-portable.zip\n")
	sig := servers.Sign(sums, priv)
	if edit != nil {
		sums = edit(sums)
	}
	srv := serveFiles(t, map[string][]byte{"/i.exe": installer, "/SHA256SUMS": sums, "/SHA256SUMS.sig": sig}, nil)
	return updater.Release{Tag: "v0.3.0", Assets: map[string]string{
		asset:            srv.URL + "/i.exe",
		"SHA256SUMS":     srv.URL + "/SHA256SUMS",
		"SHA256SUMS.sig": srv.URL + "/SHA256SUMS.sig",
	}}
}

func TestDownloadInstaller(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	body := []byte("MZ installer bytes")
	r := release(t, priv, body, nil)
	dir := t.TempDir()
	path, err := updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, asset), path)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestDownloadInstaller_BadSignature(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := release(t, priv, []byte("MZ"), func(s []byte) []byte { return append(s, '\n') })
	_, err := updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, t.TempDir())
	require.ErrorIs(t, err, servers.ErrBadSignature)
}

func TestDownloadInstaller_WrongHash(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := release(t, priv, []byte("MZ"), nil)
	// Serve different bytes under the signed name.
	srv := serveFiles(t, map[string][]byte{"/i.exe": []byte("evil")}, nil)
	r.Assets[asset] = srv.URL + "/i.exe"
	dir := t.TempDir()
	_, err := updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, dir)
	require.ErrorIs(t, err, updater.ErrChecksum)
	_, statErr := os.Stat(filepath.Join(dir, asset))
	require.True(t, os.IsNotExist(statErr), "a file that failed the check is removed")
}

// A signed SHA256SUMS of an older release must not install under a newer
// tag (downgrade with a replayed, validly signed file).
func TestDownloadInstaller_SumsOfOtherVersion(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := release(t, priv, []byte("MZ"), nil)
	r.Tag = "v0.4.0"
	_, err := updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, t.TempDir())
	require.ErrorIs(t, err, updater.ErrNoInstaller)
}

func TestDownloadInstaller_NoSignedSums(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := release(t, priv, []byte("MZ"), nil)
	delete(r.Assets, "SHA256SUMS.sig") // releases before one-click updates
	_, err := updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, t.TempDir())
	require.ErrorIs(t, err, updater.ErrNoInstaller)
}

// The checksums of v0.3.0-beta.1 name "Ghostline-0.3.0-beta.1-portable.zip",
// which contains "-0.3.0-": they must not pass for v0.3.0.
func TestDownloadInstaller_BetaSumsNotForStable(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	r := release(t, priv, []byte("MZ"), nil)
	r.Tag = "v0.3.0-beta.1"
	_, err := updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, t.TempDir())
	require.ErrorIs(t, err, updater.ErrNoInstaller, "sums of v0.3.0 do not install as v0.3.0-beta.1")

	beta := []byte("MZ beta")
	sum := sha256.Sum256(beta)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + asset + "\n" +
		"0000000000000000000000000000000000000000000000000000000000000000  Ghostline-0.3.0-beta.1-portable.zip\n")
	srv := serveFiles(t, map[string][]byte{"/i.exe": beta, "/SHA256SUMS": sums, "/SHA256SUMS.sig": servers.Sign(sums, priv)}, nil)
	r = updater.Release{Tag: "v0.3.0", Assets: map[string]string{
		asset: srv.URL + "/i.exe", "SHA256SUMS": srv.URL + "/SHA256SUMS", "SHA256SUMS.sig": srv.URL + "/SHA256SUMS.sig",
	}}
	_, err = updater.DownloadInstaller(context.Background(), http.DefaultClient, r, asset, pub, t.TempDir())
	require.ErrorIs(t, err, updater.ErrNoInstaller, "sums of v0.3.0-beta.1 do not install as v0.3.0")
}
