package procs

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// stopUnit stops a systemd unit and waits until it is inactive (or failed).
func stopUnit(unit string, wait time.Duration) error {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	mgr := conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1")
	if err := mgr.Call("org.freedesktop.systemd1.Manager.StopUnit", 0, unit, "replace").Err; err != nil {
		return err
	}
	deadline := time.Now().Add(wait)
	for {
		var path dbus.ObjectPath
		state := ""
		if err := mgr.Call("org.freedesktop.systemd1.Manager.GetUnit", 0, unit).Store(&path); err == nil {
			if v, err := conn.Object("org.freedesktop.systemd1", path).GetProperty("org.freedesktop.systemd1.Unit.ActiveState"); err == nil {
				state, _ = v.Value().(string)
			}
		}
		if state == "inactive" || state == "failed" {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("systemd: %s is %q after %s", unit, state, wait)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
