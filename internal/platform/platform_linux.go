package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/secrets"
	"github.com/hashcott/ghostline/internal/startup"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
)

var errUnsupported = fmt.Errorf("platform: %w", errors.ErrUnsupported)

// Default directories of the Linux daemon (systemd StateDirectory,
// LogsDirectory and RuntimeDirectory).
const (
	daemonDataDir = "/var/lib/ghostline/data"
	daemonLogDir  = "/var/log/ghostline"
	daemonRunDir  = "/run/ghostline"
)

// New wires the Linux daemon. The system backends are stubs until L3/L4:
// connecting fails before anything changes.
func New(exe string) (Deps, error) {
	return newLinux(daemonDataDir, daemonLogDir, daemonRunDir)
}

// NewDev lays the daemon out under dir, for development and tests that
// run without root.
func NewDev(dir string) (Deps, error) {
	return newLinux(filepath.Join(dir, "data"), filepath.Join(dir, "log"), filepath.Join(dir, "run"))
}

// ClientSocket is the socket the GUI connects to: $GHOSTLINE_SOCKET for
// development, otherwise the daemon's.
func ClientSocket() string {
	if s := os.Getenv("GHOSTLINE_SOCKET"); s != "" {
		return s
	}
	return filepath.Join(daemonRunDir, "ctl.sock")
}

func newLinux(dataDir, logDir, runDir string) (Deps, error) {
	paths := store.PathsIn(dataDir, logDir)
	unwatched := func(func()) (func(), error) { return nil, errUnsupported }
	return Deps{
		Paths:  paths,
		Lock:   newFileLock(filepath.Join(runDir, "state.lock")),
		Socket: filepath.Join(runDir, "ctl.sock"),

		DNS:           sysdns.DetectLinux(paths.DataDir),
		WatchResume:   watchResume,
		SysProxy:      sysproxy.Unsupported{},
		WatchSysProxy: unwatched,
		Certs:         certstore.Unsupported{},
		Firewall:      firewall.Unsupported{},

		DPIRunner:   dpi.UnsupportedRunner{},
		DPIServices: dpi.NoServices{},
		DPIEngines:  func(func() strategies.List) []dpi.Installed { return nil },

		Startup: startup.Unsupported{},
		StartWatchdog: func(uint32, time.Time) (func() error, error) {
			return nil, errUnsupported
		},

		UserSecrets:    secrets.Unsupported{},
		MachineSecrets: secrets.Unsupported{},

		NetID:         netid.Unsupported{},
		Procs:         procs.NewLinux(),
		AttachConsole: func() {},
		UsesDaemon:    true,
	}, nil
}
