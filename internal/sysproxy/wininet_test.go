package sysproxy_test

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

type fakeAPI struct {
	cur     model.WinINETProxy
	sets    []model.WinINETProxy
	swallow bool
	qerr    error
}

func (f *fakeAPI) Query() (model.WinINETProxy, error) { return f.cur, f.qerr }
func (f *fakeAPI) Set(s model.WinINETProxy) error {
	f.sets = append(f.sets, s)
	if !f.swallow {
		f.cur = s
	}
	return nil
}

// win wraps a WinINET configuration as the neutral snapshot.
func win(w model.WinINETProxy) sysproxy.Snapshot {
	return sysproxy.Snapshot{Backend: "windows", Windows: &w}
}

func TestWinINET_SnapshotAndInfo(t *testing.T) {
	cur := model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagAutoProxyURL, AutoconfigURL: "http://pac"}
	b := sysproxy.NewWinINET(&fakeAPI{cur: cur}, nil)
	s, err := b.Snapshot("127.0.0.1:9")
	require.NoError(t, err)
	require.Equal(t, win(cur), s)
	require.Equal(t, sysproxy.Info{Supported: true}, b.Info(), "no desktop to name on Windows")
}

func TestApply_SetsAndReadsBack(t *testing.T) {
	api := &fakeAPI{cur: model.WinINETProxy{Flags: sysproxy.FlagDirect}}
	m := sysproxy.NewWinINET(api, nil)
	require.NoError(t, m.Apply("127.0.0.1:8080"))
	require.Equal(t, model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "127.0.0.1:8080", Bypass: sysproxy.BypassWinINET()}, api.cur)
	ours, err := m.IsOurs("127.0.0.1:8080")
	require.NoError(t, err)
	require.True(t, ours)
}

func TestApply_ReadsBack(t *testing.T) {
	api := &fakeAPI{swallow: true}
	err := sysproxy.NewWinINET(api, nil).Apply("127.0.0.1:8080")
	require.ErrorIs(t, err, sysproxy.ErrNotApplied)
}

func TestExisting(t *testing.T) {
	m := sysproxy.NewWinINET(nil, nil)
	s, p, has := m.Existing(win(model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "10.0.0.1:3128"}))
	require.True(t, has)
	require.Equal(t, "10.0.0.1:3128", s)
	require.Empty(t, p)
	_, p, has = m.Existing(win(model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagAutoProxyURL, AutoconfigURL: "http://wpad/x.pac"}))
	require.True(t, has)
	require.Equal(t, "http://wpad/x.pac", p)
	_, _, has = m.Existing(win(model.WinINETProxy{Flags: sysproxy.FlagDirect, Server: "10.0.0.1:3128"}))
	require.False(t, has) // a server that is not enabled does not count
	_, _, has = m.Existing(win(model.WinINETProxy{Flags: sysproxy.FlagDirect}))
	require.False(t, has)
}

func TestRestoreIfOurs(t *testing.T) {
	snap := model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagAutoProxyURL, AutoconfigURL: "http://wpad/x.pac"}
	api := &fakeAPI{cur: snap}
	m := sysproxy.NewWinINET(api, nil)
	require.NoError(t, m.Apply("127.0.0.1:8080"))
	restored, err := m.RestoreIfOurs("127.0.0.1:8080", win(snap))
	require.NoError(t, err)
	require.True(t, restored)
	require.Equal(t, snap, api.cur)

	// Another app replaced our setting: leave it alone.
	api = &fakeAPI{cur: model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "10.0.0.1:3128"}}
	m = sysproxy.NewWinINET(api, nil)
	restored, err = m.RestoreIfOurs("127.0.0.1:8080", win(snap))
	require.NoError(t, err)
	require.False(t, restored)
	require.Empty(t, api.sets)

	// Our server string but proxy turned off by the user: not ours either.
	api = &fakeAPI{cur: model.WinINETProxy{Flags: sysproxy.FlagDirect, Server: "127.0.0.1:8080"}}
	restored, err = sysproxy.NewWinINET(api, nil).RestoreIfOurs("127.0.0.1:8080", win(snap))
	require.NoError(t, err)
	require.False(t, restored)
}

func TestRestoreIfOurs_UnreadableRestores(t *testing.T) {
	snap := model.WinINETProxy{Flags: sysproxy.FlagDirect}
	api := &fakeAPI{qerr: errors.New("query failed")}
	restored, err := sysproxy.NewWinINET(api, nil).RestoreIfOurs("127.0.0.1:8080", win(snap))
	require.NoError(t, err)
	require.True(t, restored)
	require.Equal(t, []model.WinINETProxy{snap}, api.sets)
}

// A crash before Ghostline recorded its change leaves its own address in
// place: the original is unknown, so the snapshot says "direct".
func TestWinINET_SnapshotOfOwnLeftoverIsDirect(t *testing.T) {
	b := sysproxy.NewWinINET(&fakeAPI{cur: model.WinINETProxy{Flags: sysproxy.FlagDirect | sysproxy.FlagProxy, Server: "127.0.0.1:8080", Bypass: "x"}}, nil)
	s, err := b.Snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	require.Equal(t, win(model.WinINETProxy{Flags: sysproxy.FlagDirect}), s)
}
