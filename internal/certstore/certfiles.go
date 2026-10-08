package certstore

import (
	"encoding/pem"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runner runs a system tool (update-ca-trust, certutil).
type runner func(name string, args ...string) ([]byte, error)

func execRun(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return out, errors.New(name + ": " + err.Error() + ": " + strings.TrimSpace(string(out)))
	}
	return out, nil
}

// certFile is where Ghostline keeps a root in dir: only files named this
// way are ever listed or removed.
func certFile(dir, thumbprint string) string {
	return filepath.Join(dir, "ghostline-"+thumbprint+".crt")
}

// writeCert writes der as PEM, 0644, through a temp file.
func writeCert(dir string, der []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := certFile(dir, Thumbprint(der))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// removeCert deletes Ghostline's file for thumbprint; false when there was
// none.
func removeCert(dir, thumbprint string) (bool, error) {
	err := os.Remove(certFile(dir, thumbprint))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// readCerts lists Ghostline's roots in dir whose name starts with prefix.
func readCerts(dir, prefix string) ([]Cert, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "ghostline-*.crt"))
	if err != nil {
		return nil, err
	}
	var out []Cert
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		blk, _ := pem.Decode(b)
		if blk == nil {
			continue
		}
		c, err := describe(blk.Bytes)
		if err == nil && strings.HasPrefix(c.Subject, prefix) {
			out = append(out, c)
		}
	}
	return out, nil
}
