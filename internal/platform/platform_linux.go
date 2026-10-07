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

// New wires Linux. Until the daemon (L2) and the Linux backends (L3, L4)
// exist, every system change is a stub: the GUI runs for development and
// refuses to connect. Data lives under the user's config directory for now.
func New(exe string) (Deps, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Deps{}, err
	}
	paths := store.ResolvePaths(exe, base)
	unwatched := func(func()) (func(), error) { return nil, errUnsupported }
	return Deps{
		Paths: paths,
		Lock:  newFileLock(filepath.Join(paths.DataDir, "state.lock")),

		DNS:           sysdns.Unsupported{},
		WatchNetwork:  unwatched,
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
		Procs:         procs.Unsupported{},
		AttachConsole: func() {},
	}, nil
}
