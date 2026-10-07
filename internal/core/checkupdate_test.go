package core

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/updater"
	"github.com/stretchr/testify/require"
)

type checkerEnv struct {
	c        *updateChecker
	meta     *metaFile
	st       *updateState
	notified []string
}

func newChecker(t *testing.T, current string, latest func(context.Context) (updater.Release, error)) *checkerEnv {
	e := &checkerEnv{meta: &metaFile{path: filepath.Join(t.TempDir(), "meta.json")}, st: &updateState{}}
	e.c = &updateChecker{
		meta: e.meta, state: e.st, current: current, latest: latest,
		now:     func() time.Time { return now },
		onNewer: func(tag, url string) { e.notified = append(e.notified, tag) },
	}
	return e
}

func TestCheckNow_Newer(t *testing.T) {
	e := newChecker(t, "0.2.1", func(context.Context) (updater.Release, error) {
		return updater.Release{Tag: "v0.2.2", URL: "https://example/v0.2.2"}, nil
	})
	r, err := e.c.checkNow(context.Background())
	require.NoError(t, err)
	require.Equal(t, app.UpdateCheck{Current: "0.2.1", Latest: "v0.2.2", URL: "https://example/v0.2.2", Newer: true}, r)
	m := e.meta.get()
	require.Equal(t, "v0.2.2", m.LatestTag)
	require.Equal(t, now, m.LastUpdateCheck)
	tag, _ := e.st.get()
	require.Equal(t, "v0.2.2", tag)
	require.Equal(t, []string{"v0.2.2"}, e.notified)
}

func TestCheckNow_UpToDate(t *testing.T) {
	e := newChecker(t, "0.2.1", func(context.Context) (updater.Release, error) {
		return updater.Release{Tag: "v0.2.1", URL: "u"}, nil
	})
	r, err := e.c.checkNow(context.Background())
	require.NoError(t, err)
	require.False(t, r.Newer)
	require.Equal(t, "v0.2.1", r.Latest)
	require.Equal(t, now, e.meta.get().LastUpdateCheck)
	require.Empty(t, e.notified)
}

func TestCheckNow_ErrorKeepsMeta(t *testing.T) {
	e := newChecker(t, "0.2.1", func(context.Context) (updater.Release, error) { return updater.Release{}, errors.New("offline") })
	require.NoError(t, store.SaveMeta(e.meta.path, store.Meta{LatestTag: "v0.2.1"}))
	_, err := e.c.checkNow(context.Background())
	require.Error(t, err)
	m := e.meta.get()
	require.Equal(t, "v0.2.1", m.LatestTag)
	require.True(t, m.LastUpdateCheck.IsZero())
}

func TestCheckNow_ConcurrentCallsFetchOnce(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	e := newChecker(t, "0.2.1", func(context.Context) (updater.Release, error) {
		calls.Add(1)
		<-release
		return updater.Release{Tag: "v0.2.2", URL: "u"}, nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.c.checkNow(context.Background())
			require.NoError(t, err)
			require.True(t, r.Newer)
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	require.EqualValues(t, 1, calls.Load())
}

func TestScheduled_UsesSameStateAndMeta(t *testing.T) {
	e := newChecker(t, "0.2.1", func(context.Context) (updater.Release, error) {
		return updater.Release{Tag: "v0.2.2", URL: "u"}, nil
	})
	e.c.scheduled(context.Background(), true)
	require.Equal(t, "v0.2.2", e.meta.get().LatestTag)
	require.Equal(t, []string{"v0.2.2"}, e.notified)
	// Announced once per tag.
	e.c.scheduled(context.Background(), true)
	require.Equal(t, []string{"v0.2.2"}, e.notified)
}
