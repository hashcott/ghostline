package core

import (
	"context"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/startup"
	"github.com/stretchr/testify/require"
)

// recStartup records SetAutostart calls.
type recStartup struct {
	startup.Unsupported
	calls chan bool
}

func (r recStartup) SetAutostart(on bool) error { r.calls <- on; return nil }

// The task can be gone while the setting is on (an uninstall keeps the
// settings): every start puts it back in step with the setting.
func TestStart_SyncsTheAutostartTaskWithTheSetting(t *testing.T) {
	for _, on := range []bool{true, false} {
		p := testDeps(t)
		rec := recStartup{calls: make(chan bool, 4)}
		p.Startup = rec
		c := newCore(t, p, Options{})
		s := c.Settings.Get()
		s.StartWithWindows = on
		require.NoError(t, c.Settings.Save(s))

		ctx, cancel := context.WithCancel(context.Background())
		wait := c.Start(ctx)
		select {
		case got := <-rec.calls:
			require.Equal(t, on, got)
		case <-time.After(5 * time.Second):
			t.Fatal("Start did not sync the autostart task")
		}
		cancel()
		wait()
	}
}
