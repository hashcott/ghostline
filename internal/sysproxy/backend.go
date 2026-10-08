package sysproxy

import (
	"errors"

	"github.com/hashcott/ghostline/internal/model"
)

// Snapshot is the configuration Ghostline replaced.
type Snapshot = model.ProxySnapshot

// Info tells the UI which desktop's settings Ghostline changes.
type Info struct {
	Desktop   string `json:"desktop"`   // the Linux desktop ("GNOME", "KDE"); "" on Windows or when unknown
	Supported bool   `json:"supported"` // Ghostline can set the proxy here
}

var (
	// ErrNoSession: there is no graphical session to set the proxy in yet.
	ErrNoSession = errors.New("sysproxy: no graphical session")
	// ErrDesktopUnsupported: this desktop's proxy settings are not supported.
	ErrDesktopUnsupported = errors.New("sysproxy: this desktop's proxy settings are not supported")
)

// Backend snapshots, applies and restores one OS's (or desktop's) system
// proxy setting.
type Backend interface {
	// Snapshot reads the current configuration. ours is Ghostline's proxy
	// address: finding it already set means a crash left it behind, the
	// original is unknown, and the snapshot is the system's default.
	Snapshot(ours string) (Snapshot, error)
	// Existing reports a proxy server or PAC script another app enabled.
	Existing(s Snapshot) (server, pac string, has bool)
	// Apply points the system proxy at addr and checks it reads back.
	Apply(addr string) error
	// IsOurs reports whether the system proxy is enabled and set to addr.
	IsOurs(addr string) (bool, error)
	// RestoreIfOurs puts s back unless another app or the user replaced
	// Ghostline's setting. If the current setting cannot be read, it
	// restores.
	RestoreIfOurs(addr string, s Snapshot) (bool, error)
	// Watch calls onChange when the setting may have changed.
	Watch(onChange func()) (stop func(), err error)
	Info() Info
}
