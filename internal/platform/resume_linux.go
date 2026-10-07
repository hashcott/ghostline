package platform

import "github.com/godbus/dbus/v5"

// watchResume calls onResume when logind reports the end of a sleep
// (PrepareForSleep(false)).
func watchResume(onResume func()) (func(), error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	match := []dbus.MatchOption{dbus.WithMatchInterface("org.freedesktop.login1.Manager"), dbus.WithMatchMember("PrepareForSleep")}
	if err := conn.AddMatchSignal(match...); err != nil {
		conn.Close()
		return nil, err
	}
	ch := make(chan *dbus.Signal, 4)
	conn.Signal(ch)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-ch:
				if s == nil || s.Name != "org.freedesktop.login1.Manager.PrepareForSleep" || len(s.Body) != 1 {
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
		conn.Close()
	}, nil
}
