package platform

import (
	"encoding/json"
	"fmt"
	zapret2Files "github.com/hashcott/ghostline/assets/zapret2"
	"github.com/hashcott/ghostline/internal/certstore/nss"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/session"
	"github.com/hashcott/ghostline/internal/sysproxy/desktop"
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
	// Desktop-session work (system proxy, a user's NSS) runs through the
	// session agent; what waits for a login sits in the queue.
	queue := session.NewQueue(filepath.Join(paths.DataDir, "session-queue.json"))
	var sysProxy sysproxy.Backend = sysproxy.Unsupported{}
	var nssTarget certstore.Target
	var watchSessions func(func()) (func(), error)
	if sessions, err := session.NewLinux(slog.Default()); err == nil {
		sysProxy = desktop.NewBackend(sessions, queue)
		nssTarget = nss.New(sessions, queue, filepath.Join(paths.DataDir, "nss-users.json"), nss.P11KitTrust)
		handlers := map[string]func(string, json.RawMessage) error{
			"proxy.restore": desktop.QueueHandler(sessions),
			"nss.remove":    nss.QueueHandler(sessions),
		}
		drain := func(u session.User) {
			_ = queue.Drain(u, func(task string, args json.RawMessage) error {
				h, ok := handlers[task]
				if !ok {
					return fmt.Errorf("platform: no handler for queued %s", task)
				}
				return h(task, args)
			})
		}
		watchSessions = func(onNew func()) (func(), error) {
			if u, ok := sessions.Active(); ok {
				go drain(u) // logged in before the daemon started
			}
			return sessions.WatchNew(func(u session.User) { drain(u); onNew() })
		}
	}
	// Fake SNI roots: the system anchors (required), then Firefox's policy
	// and the session user's NSS database.
	var certs certstore.Store = certstore.Unsupported{}
	if anchors, err := certstore.DetectAnchors(); err == nil {
		var optional []certstore.Target
		if certstore.FirefoxInstalled() {
			optional = append(optional, certstore.NewFirefox("/etc/firefox/policies/policies.json", filepath.Join(paths.DataDir, "firefox-policy.json")))
		}
		if nssTarget != nil {
			optional = append(optional, nssTarget)
		}
		certs = certstore.NewLinux(anchors, optional...)
	}
	return Deps{
		Paths:  paths,
		Lock:   newFileLock(filepath.Join(runDir, "state.lock")),
		Socket: filepath.Join(runDir, "ctl.sock"),

		DNS:           sysdns.DetectLinux(paths.DataDir),
		WatchResume:   watchResume,
		WatchSessions: watchSessions,
		SysProxy:      sysProxy,
		Certs:         certs,
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
