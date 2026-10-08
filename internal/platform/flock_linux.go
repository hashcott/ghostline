package platform

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// fileLock is a store.Locker across processes: an exclusive flock on path.
// local serialises goroutines of this process, which share one fileLock.
type fileLock struct {
	path  string
	local sync.Mutex
	f     *os.File
}

func newFileLock(path string) *fileLock { return &fileLock{path: path} }

// Lock blocks until no other process (or other fileLock) holds path. It
// makes path's directory (/run/ghostline) when missing: --restore can run
// before the daemon ever has.
func (l *fileLock) Lock() error {
	l.local.Lock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		l.local.Unlock()
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		l.local.Unlock()
		return err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		f.Close()
		l.local.Unlock()
		return err
	}
	l.f = f
	return nil
}

// Unlock releases the lock taken by Lock.
func (l *fileLock) Unlock() error {
	f := l.f
	if f == nil {
		return nil
	}
	l.f = nil
	defer l.local.Unlock()
	err := unix.Flock(int(f.Fd()), unix.LOCK_UN)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
