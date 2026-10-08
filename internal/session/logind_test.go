package session

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeLogind struct {
	active *sessionInfo
	byUID  map[int][]sessionInfo
}

func (f *fakeLogind) ActiveSession(string) (sessionInfo, bool, error) {
	if f.active == nil {
		return sessionInfo{}, false, nil
	}
	return *f.active, true, nil
}
func (f *fakeLogind) UserSessions(uid int) ([]sessionInfo, error) { return f.byUID[uid], nil }
func (f *fakeLogind) WatchNew(func(sessionInfo)) (func(), error)  { return func() {}, nil }

var lookupMe = func(uid int) (string, string, int, []int, error) {
	return "harry", "/home/harry", 1000, []int{1000, 998}, nil
}

func TestActive_PicksGraphicalLocalSession(t *testing.T) {
	lg := &fakeLogind{active: &sessionInfo{UID: 1000, Type: "tty", Class: "user"}}
	_, ok := activeUser(lg, lookupMe)
	require.False(t, ok, "a text console is not a desktop")

	lg.active = &sessionInfo{UID: 1000, Type: "wayland", Class: "user", Desktop: "KDE", Remote: true}
	_, ok = activeUser(lg, lookupMe)
	require.False(t, ok, "remote sessions are not this seat's user")

	lg.active = &sessionInfo{UID: 1000, Type: "wayland", Class: "user", Desktop: "KDE"}
	u, ok := activeUser(lg, lookupMe)
	require.True(t, ok)
	require.Equal(t, User{UID: 1000, GID: 1000, Groups: []int{1000, 998}, Name: "harry", Home: "/home/harry", Desktop: "KDE"}, u)
}

// Restores reach a user with any live session (a runtime dir); the desktop
// comes from a graphical one when there is.
func TestByUID(t *testing.T) {
	lg := &fakeLogind{byUID: map[int][]sessionInfo{1000: {{UID: 1000, Type: "tty", Class: "user"}, {UID: 1000, Type: "x11", Class: "user", Desktop: "GNOME"}}}}
	u, ok := userByUID(lg, lookupMe, func(int) bool { return true }, 1000)
	require.True(t, ok)
	require.Equal(t, "GNOME", u.Desktop)
	_, ok = userByUID(lg, lookupMe, func(int) bool { return false }, 1000)
	require.False(t, ok, "no runtime dir: no session bus to talk to")
}
