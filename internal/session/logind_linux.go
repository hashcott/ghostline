package session

import (
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	login1     = "org.freedesktop.login1"
	login1Path = "/org/freedesktop/login1"
	login1Mgr  = "org.freedesktop.login1.Manager"
)

type dbusLogind struct{ conn *dbus.Conn }

func newLogind() (logindAPI, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	return &dbusLogind{conn: conn}, nil
}

func (l *dbusLogind) info(path dbus.ObjectPath) (sessionInfo, error) {
	obj := l.conn.Object(login1, path)
	str := func(p string) string {
		v, err := obj.GetProperty("org.freedesktop.login1.Session." + p)
		if err != nil {
			return ""
		}
		s, _ := v.Value().(string)
		return s
	}
	v, err := obj.GetProperty("org.freedesktop.login1.Session.User")
	if err != nil {
		return sessionInfo{}, err
	}
	var uid int
	if u, ok := v.Value().([]any); ok && len(u) == 2 {
		if n, ok := u[0].(uint32); ok {
			uid = int(n)
		}
	}
	remote := false
	if r, err := obj.GetProperty("org.freedesktop.login1.Session.Remote"); err == nil {
		remote, _ = r.Value().(bool)
	}
	return sessionInfo{UID: uid, Type: str("Type"), Class: str("Class"), Desktop: str("Desktop"), Remote: remote}, nil
}

func (l *dbusLogind) ActiveSession(seat string) (sessionInfo, bool, error) {
	v, err := l.conn.Object(login1, dbus.ObjectPath(login1Path+"/seat/"+seat)).GetProperty("org.freedesktop.login1.Seat.ActiveSession")
	if err != nil {
		return sessionInfo{}, false, err
	}
	so, ok := v.Value().([]any)
	if !ok || len(so) != 2 {
		return sessionInfo{}, false, nil
	}
	path, _ := so[1].(dbus.ObjectPath)
	if path == "" || path == "/" {
		return sessionInfo{}, false, nil
	}
	s, err := l.info(path)
	return s, err == nil, err
}

func (l *dbusLogind) UserSessions(uid int) ([]sessionInfo, error) {
	var userPath dbus.ObjectPath
	if err := l.conn.Object(login1, login1Path).Call(login1Mgr+".GetUser", 0, uint32(uid)).Store(&userPath); err != nil {
		return nil, err
	}
	v, err := l.conn.Object(login1, userPath).GetProperty("org.freedesktop.login1.User.Sessions")
	if err != nil {
		return nil, err
	}
	rows, _ := v.Value().([][]any)
	var out []sessionInfo
	for _, r := range rows {
		if len(r) != 2 {
			continue
		}
		if p, ok := r[1].(dbus.ObjectPath); ok {
			if s, err := l.info(p); err == nil {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// WatchNew reports logins (SessionNew from logind itself, checked against
// the name's owner: anyone may send signals on the system bus). Each new
// session is read a second later, once logind has filled in its desktop.
func (l *dbusLogind) WatchNew(onNew func(sessionInfo)) (func(), error) {
	match := []dbus.MatchOption{dbus.WithMatchSender(login1), dbus.WithMatchInterface(login1Mgr), dbus.WithMatchMember("SessionNew")}
	if err := l.conn.AddMatchSignal(match...); err != nil {
		return nil, err
	}
	ch := make(chan *dbus.Signal, 8)
	l.conn.Signal(ch)
	done := make(chan struct{})
	var mu sync.Mutex
	var timers []*time.Timer
	go func() {
		for {
			select {
			case s := <-ch:
				if s == nil || s.Name != login1Mgr+".SessionNew" || len(s.Body) != 2 || !l.fromLogind(s.Sender) {
					continue
				}
				path, ok := s.Body[1].(dbus.ObjectPath)
				if !ok {
					continue
				}
				mu.Lock()
				timers = append(timers, time.AfterFunc(time.Second, func() {
					if si, err := l.info(path); err == nil {
						onNew(si)
					}
				}))
				mu.Unlock()
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		l.conn.RemoveSignal(ch)
		_ = l.conn.RemoveMatchSignal(match...)
		mu.Lock()
		for _, t := range timers {
			t.Stop()
		}
		mu.Unlock()
	}, nil
}

func (l *dbusLogind) fromLogind(sender string) bool {
	var owner string
	if err := l.conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, login1).Store(&owner); err != nil {
		return false
	}
	return sender != "" && sender == owner
}
