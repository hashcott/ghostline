package desktop

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/session"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

type call struct {
	uid  int
	task string
	in   string
}

// fakeSessions answers agent tasks from a script and records each call.
type fakeSessions struct {
	active  *session.User
	present map[int]session.User
	calls   []call
	answer  func(task string) any
}

func (f *fakeSessions) Active() (session.User, bool) {
	if f.active == nil {
		return session.User{}, false
	}
	return *f.active, true
}
func (f *fakeSessions) ByUID(uid int) (session.User, bool) { u, ok := f.present[uid]; return u, ok }
func (f *fakeSessions) Run(u session.User, task string, in, out any) error {
	b, _ := json.Marshal(in)
	f.calls = append(f.calls, call{u.UID, task, string(b)})
	if f.answer != nil && out != nil {
		r, _ := json.Marshal(f.answer(task))
		return json.Unmarshal(r, out)
	}
	return nil
}
func (f *fakeSessions) Stream(session.User, string, any, func(json.RawMessage)) (func(), error) {
	return func() {}, nil
}
func (f *fakeSessions) WatchNew(func(session.User)) (func(), error) { return func() {}, nil }

var kdeUser = session.User{UID: 1000, Name: "harry", Desktop: "KDE"}

func TestLinuxProxy_SnapshotNoSession(t *testing.T) {
	b := NewBackend(&fakeSessions{}, session.NewQueue(filepath.Join(t.TempDir(), "q.json")))
	_, err := b.Snapshot("127.0.0.1:8080")
	require.ErrorIs(t, err, sysproxy.ErrNoSession)
	u := session.User{UID: 1000, Desktop: "XFCE"}
	b = NewBackend(&fakeSessions{active: &u}, session.NewQueue(filepath.Join(t.TempDir(), "q.json")))
	_, err = b.Snapshot("127.0.0.1:8080")
	require.ErrorIs(t, err, sysproxy.ErrDesktopUnsupported)
}

func TestLinuxProxy_SnapshotRecordsTheUser(t *testing.T) {
	f := &fakeSessions{active: &kdeUser, answer: func(string) any {
		return model.ProxySnapshot{Backend: "kde", KDE: &model.KDEProxy{}}
	}}
	s, err := NewBackend(f, session.NewQueue(filepath.Join(t.TempDir(), "q.json"))).Snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	require.Equal(t, 1000, s.UID)
	require.Equal(t, []call{{1000, "proxy.snapshot", `{"ours":"127.0.0.1:8080"}`}}, f.calls)
}

// Review Focus 2: user 1001 is at the screen now, but the proxy was set
// for 1000: the restore goes to 1000.
func TestLinuxProxy_RestoreUsesRecordedUID(t *testing.T) {
	other := session.User{UID: 1001, Desktop: "GNOME"}
	f := &fakeSessions{active: &other, present: map[int]session.User{1000: kdeUser, 1001: other}, answer: func(string) any { return true }}
	restored, err := NewBackend(f, session.NewQueue(filepath.Join(t.TempDir(), "q.json"))).
		RestoreIfOurs("127.0.0.1:8080", model.ProxySnapshot{Backend: "kde", UID: 1000, KDE: &model.KDEProxy{}})
	require.NoError(t, err)
	require.True(t, restored)
	for _, c := range f.calls {
		require.Equal(t, 1000, c.uid, c.task)
	}
	require.Equal(t, "proxy.restore", f.calls[len(f.calls)-1].task)
}

func TestLinuxProxy_RestoreQueuesWhenUserAway(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.json")
	f := &fakeSessions{}
	snap := model.ProxySnapshot{Backend: "kde", UID: 1000, KDE: &model.KDEProxy{}}
	restored, err := NewBackend(f, session.NewQueue(path)).RestoreIfOurs("127.0.0.1:8080", snap)
	require.NoError(t, err)
	require.True(t, restored, "the queue owns it now")
	require.Empty(t, f.calls)

	// 1000 logs in: the queued restore runs, only if the setting is still ours.
	f.present = map[int]session.User{1000: kdeUser}
	f.answer = func(task string) any { return task == "proxy.isOurs" }
	require.NoError(t, session.NewQueue(path).Drain(kdeUser, QueueHandler(f)))
	require.Equal(t, []string{"proxy.isOurs", "proxy.restore"}, []string{f.calls[0].task, f.calls[1].task})
}

func TestLinuxProxy_Existing(t *testing.T) {
	b := NewBackend(&fakeSessions{}, nil)
	server, _, has := b.Existing(model.ProxySnapshot{Backend: "kde", KDE: &model.KDEProxy{Values: map[string]string{"ProxyType": "1", "httpProxy": "http://10.0.0.1 3128"}}})
	require.True(t, has)
	require.Equal(t, "10.0.0.1:3128", server)
	_, pac, has := b.Existing(model.ProxySnapshot{Backend: "gnome", GNOME: &model.GNOMEProxy{Values: map[string]string{
		"org.gnome.system.proxy mode": "'auto'", "org.gnome.system.proxy autoconfig-url": "'http://wpad/x.pac'"}}})
	require.True(t, has)
	require.Equal(t, "http://wpad/x.pac", pac)
	_, _, has = b.Existing(model.ProxySnapshot{Backend: "gnome", GNOME: &model.GNOMEProxy{}})
	require.False(t, has)
}

func TestLinuxProxy_Info(t *testing.T) {
	require.Equal(t, sysproxy.Info{Desktop: "KDE", Supported: true}, NewBackend(&fakeSessions{active: &kdeUser}, nil).Info())
	require.Equal(t, sysproxy.Info{}, NewBackend(&fakeSessions{}, nil).Info())
}

// Review I3: the proxy belongs to the user it was snapshotted for. After a
// switch to user B, Apply and IsOurs still talk to A — never to B.
func TestLinuxProxy_ApplyAndIsOursUseSnapshotUser(t *testing.T) {
	a, b := kdeUser, session.User{UID: 1001, Desktop: "KDE"}
	f := &fakeSessions{active: &a, present: map[int]session.User{1000: a, 1001: b}, answer: func(task string) any {
		if task == "proxy.snapshot" {
			return model.ProxySnapshot{Backend: "kde", KDE: &model.KDEProxy{}}
		}
		return true
	}}
	be := NewBackend(f, session.NewQueue(filepath.Join(t.TempDir(), "q.json")))
	_, err := be.Snapshot("127.0.0.1:8080")
	require.NoError(t, err)
	f.active = &b // A switched to B (e.g. during the override prompt)
	require.NoError(t, be.Apply("127.0.0.1:8080"))
	_, err = be.IsOurs("127.0.0.1:8080")
	require.NoError(t, err)
	for _, c := range f.calls {
		require.Equal(t, 1000, c.uid, c.task)
	}
}
