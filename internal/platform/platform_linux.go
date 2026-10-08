package platform

import (
	zapret2Files "github.com/hashcott/ghostline/assets/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/session"
	"log/slog"
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
	paths.BinDir = filepath.Join(filepath.Dir(dataDir), "bin") // outside data/ (0700): nfqws2 runs as nobody
	secretKey := secrets.NewFileKey(filepath.Join(dataDir, "secret.key"))
	var watchSessions func(func()) (func(), error)
	if sessions, err := session.NewLinux(slog.Default()); err == nil {
		watchSessions = func(onNew func()) (func(), error) {
			return sessions.WatchNew(func(session.User) { onNew() })
		}
	}
	return Deps{
		Paths:  paths,
		Lock:   newFileLock(filepath.Join(runDir, "state.lock")),
		Socket: filepath.Join(runDir, "ctl.sock"),

		DNS:           sysdns.DetectLinux(paths.DataDir),
		WatchResume:   watchResume,
		WatchSessions: watchSessions,
		SysProxy:      sysproxy.Unsupported{},
		Certs:         certstore.Unsupported{},
		Firewall:      firewall.Unsupported{},

		DPIRunner:      dpi.NewLinuxRunner(),
		DPIInterceptor: dpi.NewNftables(zapret2.Filter),
		DPIEngines: func(list func() strategies.List) []dpi.Installed {
			return []dpi.Installed{{Engine: zapret2.New(list), Assets: zapret2Files.FS}}
		},

		// systemd is the watchdog (ExecStopPost=--restore) and the boot
		// restore, so Connect's safety step has nothing to start.
		Startup: startup.ServiceManaged{},
		StartWatchdog: func(uint32, time.Time) (func() error, error) {
			return func() error { return nil }, nil
		},

		UserSecrets:    secretKey,
		MachineSecrets: secretKey,

		NetID:         netid.Unsupported{},
		Procs:         procs.NewLinux(),
		AttachConsole: func() {},
		UsesDaemon:    true,
	}, nil
}
