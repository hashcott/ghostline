package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	goodbyedpiFiles "github.com/hashcott/ghostline/assets/goodbyedpi"
	zapret2Files "github.com/hashcott/ghostline/assets/zapret2"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/goodbyedpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/netid"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/secrets"
	"github.com/hashcott/ghostline/internal/startup"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/hashcott/ghostline/internal/winutil"
)

// New wires Windows: everything runs in the elevated GUI process.
func New(exe string) (Deps, error) {
	lock, err := winutil.NewNamedMutex(brand.StateMutex)
	if err != nil {
		return Deps{}, err
	}
	return Deps{
		Paths: store.WithMachineDir(store.ResolvePaths(exe, os.Getenv("APPDATA")), filepath.Join(os.Getenv("ProgramData"), brand.AppName)),
		Lock:  lock,

		DNS:           sysdns.NewAdapterBackend(sysdns.NewWindowsAPI(), sysdns.Watch),
		SysProxy:      sysproxy.NewWindowsAPI(),
		WatchSysProxy: sysproxy.Watch,
		Certs:         certstore.NewWindows(certstore.LocalMachine),
		Firewall:      firewall.NewNetsh(exe),

		DPIRunner:   dpi.NewWindowsRunner(),
		DPIServices: dpi.NewWindowsServices(),
		DPIEngines: func(list func() strategies.List) []dpi.Installed {
			return []dpi.Installed{
				{Engine: goodbyedpi.New(), Assets: goodbyedpiFiles.FS},
				{Engine: zapret2.New(list), Assets: zapret2Files.FS},
			}
		},

		Startup:       startup.NewTaskScheduler(exe),
		StartWatchdog: detachedWatchdog(exe),

		UserSecrets:    secrets.NewUserDPAPI(),
		MachineSecrets: secrets.NewMachineDPAPI(),
		SecureDir:      winutil.SecureDir,
		OwnedByAdmins:  winutil.OwnedByAdmins,

		NetID:         netid.NewWindows(),
		Procs:         procs.NewWindows(),
		AttachConsole: func() { winutil.AttachParentConsole() },
	}, nil
}

// detachedWatchdog starts exe --watchdog for the given parent.
func detachedWatchdog(exe string) func(pid uint32, start time.Time) (func() error, error) {
	return func(pid uint32, start time.Time) (func() error, error) {
		// Deliberately not in our job object, and broken away from any job we
		// inherited from a terminal or IDE: it must outlive us.
		cmd, err := winutil.StartDetached(exe, []string{"--watchdog", "--parent", strconv.FormatUint(uint64(pid), 10),
			"--parent-start", strconv.FormatInt(start.UnixNano(), 10)})
		if err != nil {
			return nil, err
		}
		go func() { _ = cmd.Wait() }()
		return func() error { return cmd.Process.Kill() }, nil
	}
}
