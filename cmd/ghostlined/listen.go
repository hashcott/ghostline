//go:build linux

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"time"
)

// listen opens the control socket. A socket file left by a killed daemon
// is replaced; a live daemon on it is an error. Everyone may connect: the
// server checks each peer (SO_PEERCRED).
func listen(socket string) (net.Listener, error) {
	if c, err := net.DialTimeout("unix", socket, 500*time.Millisecond); err == nil {
		c.Close()
		return nil, fmt.Errorf("another ghostlined is running on %s", socket)
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o755); err != nil {
		return nil, err
	}
	if err := os.Remove(socket); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0o666); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}
