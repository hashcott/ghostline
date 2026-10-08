package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
)

// fileKey encrypts with AES-256-GCM under a key kept in a root-only file
// (Linux: the daemon's data directory). A blob is a 12-byte nonce followed
// by the ciphertext.
type fileKey struct {
	path string
	mu   sync.Mutex
	aead cipher.AEAD
}

// NewFileKey uses the key at path, creating it (32 random bytes, mode
// 0600) on first use.
func NewFileKey(path string) Protector { return &fileKey{path: path} }

func (k *fileKey) cipher() (cipher.AEAD, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.aead != nil {
		return k.aead, nil
	}
	key, err := os.ReadFile(k.path)
	if errors.Is(err, fs.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(k.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			return k.cipherFromFileLocked() // another process won the race
		}
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(key); err != nil {
			f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return k.setKeyLocked(key)
}

func (k *fileKey) cipherFromFileLocked() (cipher.AEAD, error) {
	key, err := os.ReadFile(k.path)
	if err != nil {
		return nil, err
	}
	return k.setKeyLocked(key)
}

func (k *fileKey) setKeyLocked(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets: %s is not a 32-byte key", k.path)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	k.aead = aead
	return aead, nil
}

func (k *fileKey) Protect(plain []byte) ([]byte, error) {
	aead, err := k.cipher()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, nil), nil
}

func (k *fileKey) Unprotect(blob []byte) ([]byte, error) {
	aead, err := k.cipher()
	if err != nil {
		return nil, err
	}
	if len(blob) < aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("secrets: blob too short")
	}
	n := aead.NonceSize()
	plain, err := aead.Open(nil, blob[:n], blob[n:], nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: %w", err)
	}
	return plain, nil
}
