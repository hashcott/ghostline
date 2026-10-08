package shell

import (
	"sync"

	"github.com/hashcott/ghostline/internal/store"
)

// settingsCache is a trayBackend whose GetSettings never calls the
// daemon: the tray reads settings on the GTK main thread and from event
// handlers. It loads once, follows "settings" events (set) and its own
// saves.
type settingsCache struct {
	trayBackend
	mu sync.Mutex
	s  store.Settings
}

func newSettingsCache(b trayBackend) *settingsCache {
	return &settingsCache{trayBackend: b, s: b.GetSettings()}
}

func (c *settingsCache) GetSettings() store.Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.s
}

func (c *settingsCache) SaveSettings(n store.Settings) error {
	if err := c.trayBackend.SaveSettings(n); err != nil {
		return err
	}
	c.set(n)
	return nil
}

// set records settings the daemon announced.
func (c *settingsCache) set(n store.Settings) {
	c.mu.Lock()
	c.s = n
	c.mu.Unlock()
}
