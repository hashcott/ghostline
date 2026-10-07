package platform

import (
	"os"

	"golang.org/x/sys/unix"
)

// fileLock is a store.Locker across processes: an exclusive flock on path.
type fileLock struct {
	path string
	f    *os.File
}

func newFileLock(path string) *fileLock { return &fileLock{path: path} }

// Lock blocks until no other process (or other fileLock) holds path.
func (l *fileLock) Lock() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		return err
	}
	l.f = f
	return nil
}

// Unlock releases the lock taken by Lock.
func (l *fileLock) Unlock() error {
	f := l.f
	l.f = nil
	if f == nil {
		return nil
	}
	err := unix.Flock(int(f.Fd()), unix.LOCK_UN)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
