package core

import (
	"io"
	"log/slog"
	"os"

	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/store"
)

// OpenLog creates the data directory and the rotating log in paths.LogDir.
func OpenLog(paths store.Paths) (*slog.Logger, io.Closer, error) {
	// The daemon's data directory holds secret.key and the LAN CA key:
	// root only, even if it was created wider before.
	if err := os.MkdirAll(paths.DataDir, 0o700); err != nil {
		return nil, nil, err
	}
	if fi, err := os.Stat(paths.DataDir); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(paths.DataDir, 0o700); err != nil {
			return nil, nil, err
		}
	}
	w, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewTextHandler(w, nil)), w, nil
}
