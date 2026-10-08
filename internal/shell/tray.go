package shell

import (
	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/store"
)

// trayBackend is what the window and tray use: app.Service methods only,
// so the in-process Service (Windows) and the daemon's proxy (Linux) both
// fit.
type trayBackend interface {
	GetSnapshot() app.Snapshot
	GetSettings() store.Settings
	SaveSettings(store.Settings) error
	Connect() error
	Disconnect() error
	SetDPIEnabled(on bool) error
	SetProxyEnabled(on bool) error
	CheckUpdateNow() (app.UpdateCheck, error)
	LANDNSClients() int
}

// trayToggle connects, or disconnects once confirm agrees when LAN
// devices use this PC's DNS: they lose the internet with it.
func trayToggle(b trayBackend, confirm func(clients int) bool) error {
	if st := b.GetSnapshot().Status; st != app.StatusProtected && st != app.StatusDegraded {
		return b.Connect()
	}
	if n := b.LANDNSClients(); n > 0 && !confirm(n) {
		return nil
	}
	return b.Disconnect()
}
