package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/updater"
	"github.com/stretchr/testify/require"
)

type selfUpdateEnv struct {
	u         *selfUpdater
	checker   *checkerEnv
	ran       []string
	runErr    error
	connected bool
}

func newSelfUpdate(t *testing.T) *selfUpdateEnv {
	e := &selfUpdateEnv{checker: newChecker(t, "0.2.1", nil)}
	dir := filepath.Join(t.TempDir(), "update")
	e.u = &selfUpdater{
		checker: e.checker.c,
		dir:     dir,
		prepare: func(string) error { return nil },
		byTag: func(_ context.Context, tag string) (updater.Release, error) {
			return updater.Release{Tag: tag}, nil
		},
		download: func(_ context.Context, r updater.Release, d string) (string, error) {
			return filepath.Join(d, "setup-"+r.Tag+".exe"), nil
		},
		connected: func() bool { return e.connected },
		run: func(path string) error {
			e.ran = append(e.ran, path)
			return e.runErr
		},
	}
	return e
}

func TestInstall_RunsTheAnnouncedRelease(t *testing.T) {
	e := newSelfUpdate(t)
	e.connected = true
	e.checker.st.set("v0.2.2", "u")
	require.NoError(t, e.u.install(context.Background()))
	require.Equal(t, []string{filepath.Join(e.u.dir, "setup-v0.2.2.exe")}, e.ran)
	require.True(t, e.checker.meta.get().ReconnectAfterUpdate)
	require.True(t, e.checker.c.takeReconnect())
	require.False(t, e.checker.c.takeReconnect(), "taken once")
}

func TestInstall_NotConnectedDoesNotReconnect(t *testing.T) {
	e := newSelfUpdate(t)
	e.checker.st.set("v0.2.2", "u")
	require.NoError(t, e.u.install(context.Background()))
	require.False(t, e.checker.meta.get().ReconnectAfterUpdate)
}

func TestInstall_NoNotice(t *testing.T) {
	e := newSelfUpdate(t)
	require.Error(t, e.u.install(context.Background()))
	require.Empty(t, e.ran)
}

func TestInstall_DownloadFailsNothingRuns(t *testing.T) {
	e := newSelfUpdate(t)
	e.connected = true
	e.checker.st.set("v0.2.2", "u")
	e.u.download = func(context.Context, updater.Release, string) (string, error) { return "", updater.ErrChecksum }
	require.ErrorIs(t, e.u.install(context.Background()), updater.ErrChecksum)
	require.Empty(t, e.ran)
	require.False(t, e.checker.meta.get().ReconnectAfterUpdate)
}

func TestInstall_RunFailsClearsReconnect(t *testing.T) {
	e := newSelfUpdate(t)
	e.connected = true
	e.checker.st.set("v0.2.2", "u")
	e.runErr = errors.New("blocked")
	require.Error(t, e.u.install(context.Background()))
	require.False(t, e.checker.meta.get().ReconnectAfterUpdate)
}

func TestInstall_OneAtATime(t *testing.T) {
	e := newSelfUpdate(t)
	e.checker.st.set("v0.2.2", "u")
	started, release := make(chan struct{}), make(chan struct{})
	e.u.download = func(_ context.Context, r updater.Release, d string) (string, error) {
		close(started)
		<-release
		return filepath.Join(d, "x.exe"), nil
	}
	done := make(chan error)
	go func() { done <- e.u.install(context.Background()) }()
	<-started
	require.ErrorIs(t, e.u.install(context.Background()), errInstallBusy)
	close(release)
	require.NoError(t, <-done)
}
