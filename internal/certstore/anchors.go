package certstore

import (
	"crypto/x509"
	"errors"
	"os"
	"os/exec"
)

// anchors is the system trust store: a root dropped in the distribution's
// anchors directory, then its rebuild tool.
type anchors struct {
	dir, tool string
	run       runner
	bundles   []string // the extracted trust bundle, first one found
}

// systemBundles are where distributions put the extracted trust bundle
// (the files Go's crypto/x509 reads on Linux).
var systemBundles = []string{
	"/etc/ssl/certs/ca-certificates.crt",                // Debian, Ubuntu, Arch, SteamOS
	"/etc/pki/tls/certs/ca-bundle.crt",                  // Fedora
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem", // Fedora
	"/etc/ssl/cert.pem",
}

// NewAnchors keeps roots in dir and runs tool after each change.
func NewAnchors(dir, tool string, run runner) Target {
	return anchors{dir: dir, tool: tool, run: run, bundles: systemBundles}
}

// Roots reads the system trust bundle now. Go's x509.SystemCertPool is
// read once per process, so a CA added since the daemon started would
// look untrusted.
func (a anchors) Roots() (*x509.CertPool, error) {
	for _, p := range a.bundles {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(b)
		return pool, nil
	}
	return nil, errors.New("certstore: no system trust bundle")
}

// anchorsFor picks the distribution's anchors directory by its tool:
// Debian/Ubuntu's update-ca-certificates, else update-ca-trust (Fedora's
// /etc/pki layout, or Arch's and SteamOS's).
func anchorsFor(hasTool func(string) bool, exists func(string) bool) (dir, tool string, ok bool) {
	switch {
	case hasTool("update-ca-certificates"):
		return "/usr/local/share/ca-certificates", "update-ca-certificates", true
	case hasTool("update-ca-trust"):
		if exists("/etc/pki/ca-trust/source/anchors") {
			return "/etc/pki/ca-trust/source/anchors", "update-ca-trust", true
		}
		return "/etc/ca-certificates/trust-source/anchors", "update-ca-trust", true
	}
	return "", "", false
}

// ErrNoTrustTool: neither update-ca-certificates nor update-ca-trust.
var ErrNoTrustTool = errors.New("certstore: no update-ca-certificates or update-ca-trust")

// DetectAnchors is this machine's system trust store.
func DetectAnchors() (Target, error) {
	dir, tool, ok := anchorsFor(
		func(t string) bool { _, err := exec.LookPath(t); return err == nil },
		func(p string) bool { _, err := os.Stat(p); return err == nil })
	if !ok {
		return nil, ErrNoTrustTool
	}
	return NewAnchors(dir, tool, execRun), nil
}

func (anchors) Name() string { return "system" }

func (a anchors) Install(der []byte) error {
	if _, err := writeCert(a.dir, der); err != nil {
		return err
	}
	_, err := a.run(a.tool)
	return err
}

func (a anchors) Remove(thumbprint string) error {
	removed, err := removeCert(a.dir, thumbprint)
	if err != nil || !removed {
		return err
	}
	_, err = a.run(a.tool)
	return err
}

func (a anchors) List(prefix string) ([]Cert, error) { return readCerts(a.dir, prefix) }
