// Package secrets encrypts small values bound to this machine (upstream
// proxy passwords in settings.json, the LAN CA key). Each OS has its own
// Protector.
package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
)

// Protector encrypts and decrypts bytes.
type Protector interface {
	Protect(plain []byte) ([]byte, error)
	Unprotect(blob []byte) ([]byte, error)
}

// EncodeString protects s and returns it base64-encoded, the format
// settings.json keeps in passEnc.
func EncodeString(p Protector, s string) (string, error) {
	b, err := p.Protect([]byte(s))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// DecodeString reverses EncodeString.
func DecodeString(p Protector, b64 string) (string, error) {
	blob, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("secrets: %w", err)
	}
	if len(blob) == 0 {
		return "", errors.New("secrets: empty blob")
	}
	plain, err := p.Unprotect(blob)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Unsupported is the Protector for an OS without support yet.
type Unsupported struct{}

var errUnsupported = fmt.Errorf("secrets: %w", errors.ErrUnsupported)

func (Unsupported) Protect([]byte) ([]byte, error)   { return nil, errUnsupported }
func (Unsupported) Unprotect([]byte) ([]byte, error) { return nil, errUnsupported }
