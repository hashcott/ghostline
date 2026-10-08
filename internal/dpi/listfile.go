package dpi

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// maxEngineList caps a list file read back from the engine's directory.
const maxEngineList = 4 << 20

// readEngineList reads a list the engine wrote. On Linux the engine runs as
// nobody and owns auto/, so the file may have been replaced by a link (to
// /etc/shadow), a pipe or something huge: only a regular file is read,
// never through a link, and at most maxEngineList bytes.
func readEngineList(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("dpi: %s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(fi, opened) {
		return nil, fmt.Errorf("dpi: %s was replaced while being read", path)
	}
	return io.ReadAll(io.LimitReader(f, maxEngineList))
}

// writeEngineList replaces path with a new regular file holding b. Creating
// with O_EXCL never follows a link the engine may have left there.
func writeEngineList(path string, b []byte) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
