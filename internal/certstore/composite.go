package certstore

import (
	"crypto/x509"
	"errors"
	"fmt"
	"strings"
)

// Target is one place a root certificate is trusted from (Linux: the
// system anchors, Firefox's policy, a user's NSS database).
type Target interface {
	Store
	Name() string
}

// PartialError: the required target has the certificate, but optional
// ones (Targets) failed. Callers treat it as installed, with a warning.
type PartialError struct {
	Targets []string
	Err     error
}

func (e *PartialError) Error() string {
	return fmt.Sprintf("certstore: not in %s: %v", strings.Join(e.Targets, ", "), e.Err)
}
func (e *PartialError) Unwrap() error { return e.Err }

// RootsSource reads the system's trusted roots afresh (Linux), for checks
// that must see a root added a moment ago. A nil pool means "use the
// system default".
type RootsSource interface {
	Roots() (*x509.CertPool, error)
}

type composite struct {
	required Target
	optional []Target
}

// NewLinux trusts a root in every target. The required one must succeed;
// optional failures make a *PartialError.
func NewLinux(required Target, optional ...Target) Store {
	return composite{required: required, optional: optional}
}

func partial(names []string, errs []error) error {
	if len(names) == 0 {
		return nil
	}
	return &PartialError{Targets: names, Err: errors.Join(errs...)}
}

func (c composite) Install(der []byte) error {
	if err := c.required.Install(der); err != nil {
		return err
	}
	var names []string
	var errs []error
	for _, t := range c.optional {
		if err := t.Install(der); err != nil {
			names, errs = append(names, t.Name()), append(errs, err)
		}
	}
	return partial(names, errs)
}

func (c composite) Remove(thumbprint string) error {
	var names []string
	var errs []error
	for _, t := range c.optional {
		if err := t.Remove(thumbprint); err != nil {
			names, errs = append(names, t.Name()), append(errs, err)
		}
	}
	if err := c.required.Remove(thumbprint); err != nil {
		return errors.Join(append(errs, err)...)
	}
	return partial(names, errs)
}

// Roots comes from the required target when it can read them afresh.
func (c composite) Roots() (*x509.CertPool, error) {
	if rs, ok := c.required.(RootsSource); ok {
		return rs.Roots()
	}
	return nil, nil
}

// List is every target's certificates, once each.
func (c composite) List(prefix string) ([]Cert, error) {
	seen := map[string]bool{}
	var out []Cert
	var errs []error
	for _, t := range append([]Target{c.required}, c.optional...) {
		l, err := t.List(prefix)
		errs = append(errs, err)
		for _, cert := range l {
			if !seen[cert.Thumbprint] {
				seen[cert.Thumbprint] = true
				out = append(out, cert)
			}
		}
	}
	return out, errors.Join(errs...)
}

// ErrNSSToolMissing: a user's NSS database needs certutil, which is not
// installed (Ubuntu: libnss3-tools).
var ErrNSSToolMissing = errors.New("certstore: certutil not found")
