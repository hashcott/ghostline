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
	if err := os.MkdirAll(paths.DataDir, 0o755); err != nil {
		return nil, nil, err
	}
	w, err := logx.NewRotating(paths.LogDir, "ghostline", 5<<20, 3)
	if err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewTextHandler(w, nil)), w, nil
}
