package shell

import (
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type countingTray struct {
	fakeTray
	gets  int
	saved []store.Settings
	s     store.Settings
}

func (c *countingTray) GetSettings() store.Settings { c.gets++; return c.s }
func (c *countingTray) SaveSettings(n store.Settings) error {
	c.saved = append(c.saved, n)
	c.s = n
	return nil
}

// C1: the tray reads settings on the GTK main thread and from event
// handlers; on Linux that must never be a call to the daemon.
func TestSettingsCache_ReadsWithoutCalling(t *testing.T) {
	b := &countingTray{s: store.DefaultSettings()}
	c := newSettingsCache(b)
	require.Equal(t, 1, b.gets, "loaded once")
	require.Equal(t, "vi", c.GetSettings().Language)
	require.Equal(t, "vi", c.GetSettings().Language)
	require.Equal(t, 1, b.gets)

	n := store.DefaultSettings()
	n.Language = "en"
	require.NoError(t, c.SaveSettings(n))
	require.Equal(t, "en", c.GetSettings().Language)
	require.Len(t, b.saved, 1)

	n.Language = "vi"
	c.set(n) // a "settings" event
	require.Equal(t, "vi", c.GetSettings().Language)
	require.Equal(t, 1, b.gets)
}
