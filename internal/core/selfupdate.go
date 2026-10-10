package core

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/updater"
)

var (
	errNoUpdate    = errors.New("no newer release is announced")
	errInstallBusy = errors.New("an update is already being installed")
)

// selfUpdater installs the announced release from the app: download the
// installer, check it against the signed SHA256SUMS, start it. The host's
// run starts the installer and quits Ghostline; the installer reopens it.
type selfUpdater struct {
	checker *updateChecker
	// dir holds the downloaded installer. It is emptied first and prepared
	// so that only administrators can write to it: the installer runs
	// elevated, and a file a normal program could swap after the check
	// would run as administrator.
	dir       string
	prepare   func(dir string) error
	byTag     func(ctx context.Context, tag string) (updater.Release, error)
	download  func(ctx context.Context, r updater.Release, dir string) (string, error)
	connected func() bool
	run       func(path string) error

	mu   sync.Mutex
	busy bool
}

// newSelfUpdater wires GitHub, the release key and the host's run.
func newSelfUpdater(checker *updateChecker, paths store.Paths, secureDir func(string) error, connected func() bool, run func(string) error) *selfUpdater {
	client := &http.Client{} // the context bounds the whole download
	prepare := func(dir string) error { return os.MkdirAll(dir, 0o700) }
	if secureDir != nil {
		prepare = secureDir
	}
	return &selfUpdater{
		checker: checker,
		dir:     filepath.Join(paths.MachineDir, "update"),
		prepare: prepare,
		byTag: func(ctx context.Context, tag string) (updater.Release, error) {
			return updater.ByTag(ctx, client, brand.ReleasesAPI, tag)
		},
		download: func(ctx context.Context, r updater.Release, dir string) (string, error) {
			return updater.DownloadInstaller(ctx, client, r, brand.InstallerAsset, serverListKey(), dir)
		},
		connected: connected,
		run:       run,
	}
}

// install downloads, checks and starts the installer of the announced
// release. When Ghostline is connected it remembers to connect again after
// the update.
func (u *selfUpdater) install(ctx context.Context) error {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return errInstallBusy
	}
	u.busy = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()

	tag, _ := u.checker.state.get()
	if tag == "" {
		return errNoUpdate
	}
	r, err := u.byTag(ctx, tag)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(u.dir); err != nil {
		return err
	}
	if err := u.prepare(u.dir); err != nil {
		return err
	}
	path, err := u.download(ctx, r, u.dir)
	if err != nil {
		return err
	}
	reconnect := u.connected()
	u.checker.meta.update(func(m *store.Meta) { m.ReconnectAfterUpdate = reconnect })
	if err := u.run(path); err != nil {
		u.checker.meta.update(func(m *store.Meta) { m.ReconnectAfterUpdate = false })
		return err
	}
	return nil
}
