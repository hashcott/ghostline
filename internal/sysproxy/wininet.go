// Package sysproxy snapshots, applies and restores the system proxy: the
// WinINET per-connection options on Windows, the desktop's settings on
// Linux (GNOME, KDE).
package sysproxy

import (
	"errors"

	"github.com/hashcott/ghostline/internal/model"
)

// API reads and writes the WinINET per-connection proxy options. Set must
// also broadcast the settings change so running browsers pick it up.
type API interface {
	Query() (model.WinINETProxy, error)
	Set(model.WinINETProxy) error
}

// INTERNET_PER_CONN_FLAGS bits.
const (
	FlagDirect       uint32 = 1
	FlagProxy        uint32 = 2
	FlagAutoProxyURL uint32 = 4
)

// ErrNotApplied means the setting did not read back as written.
var ErrNotApplied = errors.New("sysproxy: setting did not stick")

// winINET is the Backend over the WinINET options.
type winINET struct {
	api   API
	watch func(onChange func()) (func(), error)
}

// NewWinINET applies Ghostline's proxy through api and restores the
// previous one; watch reports changes to the setting.
func NewWinINET(api API, watch func(onChange func()) (func(), error)) Backend {
	return winINET{api: api, watch: watch}
}

// Snapshot reads the current configuration; Ghostline's own leftover
// address becomes "direct".
func (m winINET) Snapshot(ours string) (Snapshot, error) {
	cur, err := m.api.Query()
	if err != nil {
		return Snapshot{}, err
	}
	if cur.Server == ours {
		cur = model.WinINETProxy{Flags: FlagDirect}
	}
	return Snapshot{Backend: "windows", Windows: &cur}, nil
}

// Existing reports a proxy server or PAC script another app has enabled.
func (winINET) Existing(snap Snapshot) (server, pac string, has bool) {
	if snap.Windows == nil {
		return "", "", false
	}
	s := *snap.Windows
	if s.Flags&FlagProxy != 0 && s.Server != "" {
		server = s.Server
	}
	if s.Flags&FlagAutoProxyURL != 0 && s.AutoconfigURL != "" {
		pac = s.AutoconfigURL
	}
	return server, pac, server != "" || pac != ""
}

// Apply points the system proxy at addr and checks it reads back.
func (m winINET) Apply(addr string) error {
	if err := m.api.Set(model.WinINETProxy{Flags: FlagDirect | FlagProxy, Server: addr, Bypass: BypassWinINET()}); err != nil {
		return err
	}
	ours, err := m.IsOurs(addr)
	if err != nil {
		return err
	}
	if !ours {
		return ErrNotApplied
	}
	return nil
}

// IsOurs reports whether the system proxy is enabled and set to addr.
func (m winINET) IsOurs(addr string) (bool, error) {
	cur, err := m.api.Query()
	if err != nil {
		return false, err
	}
	return cur.Flags&FlagProxy != 0 && cur.Server == addr, nil
}

// RestoreIfOurs puts snap back unless another app or the user replaced
// Ghostline's setting. If the current setting cannot be read, it restores.
func (m winINET) RestoreIfOurs(addr string, snap Snapshot) (bool, error) {
	if snap.Windows == nil {
		return false, errors.New("sysproxy: no WinINET snapshot to restore")
	}
	if ours, err := m.IsOurs(addr); err == nil && !ours {
		return false, nil
	}
	if err := m.api.Set(*snap.Windows); err != nil {
		return false, err
	}
	return true, nil
}

// Watch reports changes to the Windows proxy settings.
func (m winINET) Watch(onChange func()) (func(), error) {
	if m.watch == nil {
		return func() {}, nil
	}
	return m.watch(onChange)
}

func (winINET) Info() Info { return Info{Supported: true} }
