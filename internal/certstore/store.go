// Package certstore installs, lists and removes Ghostline's root
// certificates in the operating system's trust store. Only the Windows
// implementation exists; Store carries no Windows types so macOS/Linux
// implementations can be added later (spec 2B 4.1).
package certstore

import (
	"crypto/sha1"
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Cert is one certificate found in the store.
type Cert struct {
	Thumbprint string    `json:"thumbprint"` // lower-case hex SHA-1
	Subject    string    `json:"subject"`    // common name
	NotAfter   time.Time `json:"notAfter"`
}

// Store is a root trust store.
type Store interface {
	// Install adds der (replacing an identical one) and reads it back.
	Install(der []byte) error
	// Remove deletes the certificate; a missing one is not an error.
	Remove(thumbprint string) error
	// List returns the certificates whose common name starts with prefix.
	List(subjectPrefix string) ([]Cert, error)
}

// ErrNotInstalled means Install could not find the certificate afterwards.
var ErrNotInstalled = errors.New("certstore: certificate not found after install")

// Thumbprint is the lower-case hex SHA-1 of a DER certificate.
func Thumbprint(der []byte) string {
	sum := sha1.Sum(der)
	return fmt.Sprintf("%x", sum[:])
}

// Sweep removes every certificate with prefix whose thumbprint is not in
// keep, and returns the removed thumbprints. It keeps going after an
// error and returns all errors joined.
func Sweep(s Store, prefix string, keep []string) ([]string, error) {
	certs, err := s.List(prefix)
	if err != nil {
		return nil, err
	}
	var removed []string
	var errs []error
	for _, c := range certs {
		if contains(keep, c.Thumbprint) {
			continue
		}
		if err := s.Remove(c.Thumbprint); err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, c.Thumbprint)
	}
	return removed, errors.Join(errs...)
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// Describe reads a certificate's thumbprint, common name and expiry.
func Describe(der []byte) (Cert, error) { return describe(der) }

func describe(der []byte) (Cert, error) {
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return Cert{}, err
	}
	return Cert{Thumbprint: Thumbprint(der), Subject: c.Subject.CommonName, NotAfter: c.NotAfter}, nil
}

// Fake is an in-memory Store for tests.
type Fake struct {
	mu          sync.Mutex
	certs       map[string][]byte
	FailInstall error
	FailRemove  error
	// OnInstall runs before an install is recorded (tests check ordering).
	OnInstall func(thumbprint string)
}

// NewFake creates an empty fake store.
func NewFake() *Fake { return &Fake{certs: map[string][]byte{}} }

// Install implements Store.
func (f *Fake) Install(der []byte) error {
	if f.OnInstall != nil {
		f.OnInstall(Thumbprint(der))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailInstall != nil {
		return f.FailInstall
	}
	f.certs[Thumbprint(der)] = append([]byte(nil), der...)
	return nil
}

// Remove implements Store.
func (f *Fake) Remove(thumbprint string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailRemove != nil {
		return f.FailRemove
	}
	delete(f.certs, strings.ToLower(thumbprint))
	return nil
}

// List implements Store.
func (f *Fake) List(prefix string) ([]Cert, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Cert
	for _, der := range f.certs {
		c, err := describe(der)
		if err == nil && strings.HasPrefix(c.Subject, prefix) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Has reports whether thumbprint is installed.
func (f *Fake) Has(thumbprint string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.certs[strings.ToLower(thumbprint)]
	return ok
}

// RemoveIfPrefix removes thumbprint only when the certificate it names is
// one of Ghostline's (common name starting with prefix). Thumbprints read
// from files a normal user can write must never delete another root.
// A thumbprint not in the store is not an error.
func RemoveIfPrefix(s Store, thumbprint, prefix string) error {
	list, err := s.List(prefix)
	if err != nil {
		return err
	}
	for _, c := range list {
		if strings.EqualFold(c.Thumbprint, thumbprint) {
			return s.Remove(c.Thumbprint)
		}
	}
	return nil
}
