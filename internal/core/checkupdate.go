package core

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/updater"
	"golang.org/x/sync/singleflight"
)

// metaFile serialises read-modify-write of meta.json between the
// background jobs and the manual update check.
type metaFile struct {
	mu   sync.Mutex
	path string
}

func (m *metaFile) get() store.Meta {
	m.mu.Lock()
	defer m.mu.Unlock()
	return store.LoadMeta(m.path)
}

func (m *metaFile) update(fn func(*store.Meta)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta := store.LoadMeta(m.path)
	fn(&meta)
	if err := store.SaveMeta(m.path, meta); err != nil {
		slog.Warn("meta: saving meta.json failed", "path", m.path, "err", err)
	}
}

// updateChecker runs release checks, scheduled or manual, through one
// path: same meta, same notice state, at most one request in flight.
type updateChecker struct {
	meta    *metaFile
	state   *updateState
	current string
	latest  func(ctx context.Context) (updater.Release, error)
	now     func() time.Time
	beta    func() bool           // pre-releases count; nil = stable only
	onNewer func(tag, url string) // UI notice (event, tray); once per tag, "" withdraws it

	sf singleflight.Group
}

// announce tells the UI about a newer release it has not seen yet.
func (c *updateChecker) announce(r updater.Release) {
	if c.state.set(r.Tag, r.URL) && c.onNewer != nil {
		c.onNewer(r.Tag, r.URL)
	}
}

// withdraw removes a notice that no longer applies (the channel went from
// beta to stable, or the announced release is now installed).
func (c *updateChecker) withdraw() {
	if c.state.set("", "") && c.onNewer != nil {
		c.onNewer("", "")
	}
}

func (c *updateChecker) betaOn() bool { return c.beta != nil && c.beta() }

// takeReconnect reports, once, whether the app started an installer while
// connected (see selfUpdater.install).
func (c *updateChecker) takeReconnect() bool {
	var on bool
	if !c.meta.get().ReconnectAfterUpdate {
		return false
	}
	c.meta.update(func(m *store.Meta) {
		on, m.ReconnectAfterUpdate = m.ReconnectAfterUpdate, false
	})
	return on
}

// checkNow asks GitHub immediately (manual "check for updates").
func (c *updateChecker) checkNow(ctx context.Context) (app.UpdateCheck, error) {
	v, err, _ := c.sf.Do("release", func() (any, error) {
		r, err := c.latest(ctx)
		if err != nil {
			return nil, err
		}
		c.meta.update(func(m *store.Meta) {
			m.LastUpdateCheck = c.now()
			m.LatestTag, m.LatestURL = r.Tag, r.URL
		})
		return r, nil
	})
	if err != nil {
		return app.UpdateCheck{}, err
	}
	r := v.(updater.Release)
	out := app.UpdateCheck{Current: c.current, Latest: r.Tag, URL: r.URL, Newer: updater.Newer(c.current, r.Tag)}
	if out.Newer {
		c.announce(r)
	} else {
		c.withdraw()
	}
	return out, nil
}

// scheduled is the automatic check: at start, every releaseInterval, and
// whenever no release is remembered (see releaseCheck).
func (c *updateChecker) scheduled(ctx context.Context, startup bool) {
	var r updater.Release
	var ok bool
	c.meta.update(func(m *store.Meta) {
		r, ok = releaseCheck(m, c.now(), c.current, startup, c.betaOn(), func() (updater.Release, error) {
			v, err, _ := c.sf.Do("release", func() (any, error) { return c.latest(ctx) })
			if err != nil {
				return updater.Release{}, err
			}
			return v.(updater.Release), nil
		})
	})
	if ok {
		c.announce(r)
	} else {
		c.withdraw()
	}
}
