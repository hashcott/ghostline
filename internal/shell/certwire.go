package shell

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/hashcott/ghostline/internal/platform"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/store"
)

// certWiring implements app.Certs over the Windows Root store and the LAN
// CA files in the data directory.
type certWiring struct {
	paths store.Paths
	store certstore.Store
	prot  certs.Protector
	host  func() string
	now   func() time.Time
	// secure makes paths.MachineDir admin-only; owned says whether a file
	// is owned by Administrators/SYSTEM. Nil skips the check (tests).
	secure func(dir string) error
	owned  func(path string) (bool, error)

	mu  sync.Mutex
	lan *certs.CA
}

func newCertWiring(p platform.Deps) *certWiring {
	return &certWiring{paths: p.Paths, store: p.Certs, prot: p.MachineSecrets,
		host: func() string { h, _ := os.Hostname(); return h }, now: time.Now,
		secure: p.SecureDir, owned: p.OwnedByAdmins}
}

// loadLocked returns the LAN CA from memory or disk; nil when none exists.
func (c *certWiring) loadLocked() (*certs.CA, error) {
	if c.lan != nil {
		return c.lan, nil
	}
	if c.secure != nil && c.paths.MachineDir != "" {
		if err := c.secure(c.paths.MachineDir); err != nil {
			return nil, err
		}
	}
	if c.owned != nil && !c.ownedByAdmins() {
		// Planted or left by a normal user: never trust it; start over.
		if err := c.removeLocked(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	ca, err := certs.LoadLANCA(c.paths.LANCACert, c.paths.LANCAKey, c.prot)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.lan = ca
	return ca, nil
}

// LANCA implements app.Certs: load or create the LAN CA, and (re)install
// it in Root (adding an identical certificate is a no-op).
func (c *certWiring) LANCA(context.Context) (*certs.CA, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ca, err := c.loadLocked()
	if err != nil {
		return nil, err
	}
	if ca == nil {
		if ca, err = certs.NewLANCA(c.host(), c.now()); err != nil {
			return nil, err
		}
		if err := certs.SaveLANCA(c.paths.LANCACert, c.paths.LANCAKey, ca, c.prot); err != nil {
			return nil, fmt.Errorf("save LAN CA: %w", err)
		}
		c.lan = ca
	}
	if err := c.store.Install(ca.DER); err != nil {
		return nil, err
	}
	return ca, nil
}

// ownedByAdmins is true when the LAN CA files are missing or both owned by
// Administrators/SYSTEM.
func (c *certWiring) ownedByAdmins() bool {
	for _, p := range []string{c.paths.LANCACert, c.paths.LANCAKey} {
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if ok, err := c.owned(p); err != nil || !ok {
			return false
		}
	}
	return true
}

// removeLocked removes the LAN CA from Root and deletes its files. An
// unreadable key still lets the certificate file be removed.
func (c *certWiring) removeLocked() error {
	var errs []error
	if der, err := os.ReadFile(c.paths.LANCACert); err == nil {
		errs = append(errs, certstore.RemoveIfPrefix(c.store, certstore.Thumbprint(der), certs.LANPrefix))
	}
	for _, p := range []string{c.paths.LANCACert, c.paths.LANCAKey} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	c.lan = nil
	return errors.Join(errs...)
}

// ResetLANCA implements app.Certs.
func (c *certWiring) ResetLANCA(ctx context.Context) (*certs.CA, error) {
	c.mu.Lock()
	err := c.removeLocked()
	c.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return c.LANCA(ctx)
}

// RemoveLANCA implements app.Certs.
func (c *certWiring) RemoveLANCA(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeLocked()
}

// InstallSession implements app.Certs.
func (c *certWiring) InstallSession(der []byte) error { return c.store.Install(der) }

// RemoveSession implements app.Certs.
// Only Fake SNI roots are removed: thumbprints may come from state.json.
func (c *certWiring) RemoveSession(thumb string) error {
	return certstore.RemoveIfPrefix(c.store, thumb, certs.SessionPrefix)
}

// List implements app.Certs.
func (c *certWiring) List() ([]certstore.Cert, error) { return c.store.List("Ghostline") }

// mitmBox holds the Fake SNI certificate source the proxy reads.
type mitmBox struct{ p atomic.Pointer[leafRef] }

type leafRef struct{ l mitm.LeafSource }

func (b *mitmBox) set(l mitm.LeafSource) {
	if l == nil {
		b.p.Store(nil)
		return
	}
	b.p.Store(&leafRef{l: l})
}

func (b *mitmBox) get() mitm.LeafSource {
	if r := b.p.Load(); r != nil {
		return r.l
	}
	return nil
}

// selfTest checks that Windows itself trusts a certificate from the
// session CA: the CA really is in Root and its Name Constraints are
// accepted (Verify with nil roots uses the system verifier on Windows).
func (b *mitmBox) selfTest(context.Context) error {
	is, ok := b.get().(*certs.Issuer)
	if !ok {
		return errors.New("fake SNI: no session CA")
	}
	domains := is.CA().Cert.PermittedDNSDomains
	if len(domains) == 0 {
		return errors.New("fake SNI: session CA has no domain")
	}
	leaf, err := is.Leaf(domains[0])
	if err != nil {
		return err
	}
	inter := x509.NewCertPool()
	_, err = leaf.Leaf.Verify(x509.VerifyOptions{DNSName: domains[0], Intermediates: inter,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil {
		return fmt.Errorf("fake SNI: the system does not trust the session CA: %w", err)
	}
	return nil
}
