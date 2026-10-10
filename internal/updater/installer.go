package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Release files that make an installer trustworthy: the checksums of every
// file and their ed25519 signature (servers.Sign format, made by the
// release workflow with the server-list key).
const (
	SumsAsset    = "SHA256SUMS"
	SumsSigAsset = "SHA256SUMS.sig"
)

var (
	// ErrNoInstaller means the release cannot be installed from the app:
	// a file is missing (releases before one-click updates have no
	// SHA256SUMS.sig) or the signed checksums are not this release's.
	ErrNoInstaller = errors.New("updater: release has no signed installer")
	// ErrChecksum means the downloaded installer is not the signed one.
	ErrChecksum = errors.New("updater: installer checksum mismatch")
)

// maxInstaller bounds the installer download.
const maxInstaller = 512 << 20

// DownloadInstaller fetches asset of r into dir and returns its path, once
// SHA256SUMS verifies with pub, names this release's version (a replayed
// file of an older release does not) and lists the downloaded bytes. A
// file that fails the check is removed.
func DownloadInstaller(ctx context.Context, c *http.Client, r Release, asset string, pub ed25519.PublicKey, dir string) (string, error) {
	sumsURL, sigURL, fileURL := r.Assets[SumsAsset], r.Assets[SumsSigAsset], r.Assets[asset]
	if sumsURL == "" || sigURL == "" || fileURL == "" {
		return "", ErrNoInstaller
	}
	sums, _, err := FetchSigned(ctx, c, sumsURL, sigURL, pub)
	if err != nil {
		return "", err
	}
	want, ok := signedSum(sums, asset, r.Tag)
	if !ok {
		return "", ErrNoInstaller
	}
	path := filepath.Join(dir, asset)
	if err := download(ctx, c, fileURL, path, want); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// signedSum finds asset's checksum in a sha256sum listing, which must also
// name a package of exactly version tag (Ghostline-0.3.0-portable.zip,
// ghostline-0.3.0-linux-amd64.tar.gz, ghostline_0.3.0_amd64.deb): a
// "-0.3.0-" alone would also match the files of 0.3.0-beta.1.
func signedSum(sums []byte, asset, tag string) ([]byte, bool) {
	ver := strings.TrimPrefix(tag, "v")
	marks := []string{"-" + ver + "-portable.", "-" + ver + "-linux-", "_" + ver + "_"}
	var want []byte
	ofTag := false
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		hash, name, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		name = strings.TrimPrefix(strings.TrimSpace(name), "*") // binary-mode marker
		for _, m := range marks {
			if strings.Contains(name, m) {
				ofTag = true
			}
		}
		if name == asset {
			if b, err := hex.DecodeString(hash); err == nil && len(b) == sha256.Size {
				want = b
			}
		}
	}
	return want, want != nil && ofTag
}

func download(ctx context.Context, c *http.Client, url, path string, want []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Ghostline")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("updater: GET %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, maxInstaller))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(h.Sum(nil), want) {
		return ErrChecksum
	}
	return nil
}
