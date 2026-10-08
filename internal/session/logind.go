package session

// sessionInfo is what logind says about a session.
type sessionInfo struct {
	UID     int
	Type    string // x11, wayland, tty, …
	Class   string // user, greeter, …
	Desktop string
	Remote  bool
}

// logindAPI is the part of systemd-logind the agent needs; logind_linux.go
// speaks D-Bus, tests use a fake.
type logindAPI interface {
	ActiveSession(seat string) (sessionInfo, bool, error)
	UserSessions(uid int) ([]sessionInfo, error)
	WatchNew(onNew func(sessionInfo)) (stop func(), err error)
}

// lookupFunc resolves a uid to its name, home, primary group and groups.
type lookupFunc func(uid int) (name, home string, gid int, groups []int, err error)

func graphical(s sessionInfo) bool {
	return (s.Type == "x11" || s.Type == "wayland") && s.Class == "user" && !s.Remote
}

func userOf(s sessionInfo, lookup lookupFunc) (User, bool) {
	name, home, gid, groups, err := lookup(s.UID)
	if err != nil {
		return User{}, false
	}
	return User{UID: s.UID, GID: gid, Groups: groups, Name: name, Home: home, Desktop: s.Desktop}, true
}

// activeUser is the owner of seat0's active graphical, local session.
func activeUser(lg logindAPI, lookup lookupFunc) (User, bool) {
	s, ok, err := lg.ActiveSession("seat0")
	if err != nil || !ok || !graphical(s) {
		return User{}, false
	}
	return userOf(s, lookup)
}

// userByUID is uid when its session bus exists (any session); the desktop
// comes from one of its graphical sessions, if any.
func userByUID(lg logindAPI, lookup lookupFunc, hasBus func(uid int) bool, uid int) (User, bool) {
	if !hasBus(uid) {
		return User{}, false
	}
	desktop := ""
	if ss, err := lg.UserSessions(uid); err == nil {
		for _, s := range ss {
			if graphical(s) {
				desktop = s.Desktop
				break
			}
		}
	}
	u, ok := userOf(sessionInfo{UID: uid, Desktop: desktop}, lookup)
	return u, ok
}
