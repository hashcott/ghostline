package platform

import "github.com/godbus/dbus/v5"

const logindName = "org.freedesktop.login1"

// watchResume calls onResume when logind reports the end of a sleep
// (PrepareForSleep(false)).
func watchResume(onResume func()) (func(), error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	stop, err := watchResumeOn(conn, onResume)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return func() { stop(); conn.Close() }, nil
}

// watchResumeOn listens on conn. Any user may send signals on the system
// bus: only those from logind's current owner count.
func watchResumeOn(conn *dbus.Conn, onResume func()) (func(), error) {
	match := []dbus.MatchOption{dbus.WithMatchSender(logindName), dbus.WithMatchInterface("org.freedesktop.login1.Manager"), dbus.WithMatchMember("PrepareForSleep")}
	if err := conn.AddMatchSignal(match...); err != nil {
		return nil, err
	}
	ch := make(chan *dbus.Signal, 4)
	conn.Signal(ch)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-ch:
				if s == nil || s.Name != "org.freedesktop.login1.Manager.PrepareForSleep" || len(s.Body) != 1 || !fromOwner(conn, s.Sender, logindName) {
					continue
				}
				if sleeping, ok := s.Body[0].(bool); ok && !sleeping {
					onResume()
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		close(done)
		conn.RemoveSignal(ch)
		_ = conn.RemoveMatchSignal(match...)
	}, nil
}

// fromOwner reports whether sender is the unique name that owns name now.
func fromOwner(conn *dbus.Conn, sender, name string) bool {
	var owner string
	if err := conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner); err != nil {
		return false
	}
	return sender != "" && sender == owner
}
