// Package platform is the one place that picks each OS's implementations.
// Every other package depends on interfaces; New (one per OS file) wires
// them, so supporting another OS means one more platform_<os>.go.
package platform

import (
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

// Deps is everything OS-specific Ghostline uses.
type Deps struct {
	// Name is the OS ("windows", "linux"), so shared code never asks
	// runtime.GOOS.
	Name string

	Paths store.Paths
	Lock  store.Locker // serialises state.json across processes
	// Socket is the daemon's control socket ("" where there is no daemon).
	Socket string

	DNS      sysdns.Backend // also watches the network
	SysProxy sysproxy.Backend
	Certs    certstore.Store
	Firewall firewall.Manager

	DPIRunner      dpi.Runner
	DPIInterceptor dpi.Interceptor
	// DPIEngines lists the engines this OS can run, with their files.
	DPIEngines func(list func() strategies.List) []dpi.Installed

	// WatchResume reports the machine waking from sleep; nil where the GUI
	// sees it itself (Windows: WM_POWERBROADCAST).
	WatchResume func(onResume func()) (stop func(), err error)
	// WatchSessions reports a user logging in (Linux: logind), so work that
	// waited for their desktop session can run; nil where there is none.
	WatchSessions func(onNew func()) (stop func(), err error)

	Startup       startup.Manager
	StartWatchdog func(pid uint32, start time.Time) (stop func() error, err error)

	UserSecrets    secrets.Protector               // upstream proxy passwords
	MachineSecrets secrets.Protector               // the LAN CA key
	SecureDir      func(dir string) error          // nil: no ACL step on this OS
	OwnedByAdmins  func(path string) (bool, error) // nil: no ownership check on this OS

	NetID         netid.Source
	Procs         procs.Inspector
	AttachConsole func() // lets a GUI binary print to the terminal that started it

	// UsesDaemon is true where a root daemon runs Ghostline and the GUI is
	// its client (Linux from L2 on).
	UsesDaemon bool
}
