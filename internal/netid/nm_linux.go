package netid

import (
	"errors"

	"github.com/godbus/dbus/v5"
)

const (
	nmName = "org.freedesktop.NetworkManager"
	nmPath = "/org/freedesktop/NetworkManager"
)

type dbusNM struct{ conn *dbus.Conn }

// newNM is NetworkManager on the system bus, or an error when it is not
// running (then there is no SSID, as on a machine without Wi-Fi tools).
func newNM() (nmAPI, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}
	var ok bool
	if err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, nmName).Store(&ok); err != nil || !ok {
		return nil, errors.New("netid: NetworkManager is not running")
	}
	return dbusNM{conn}, nil
}

func (n dbusNM) prop(path dbus.ObjectPath, name string) (dbus.Variant, error) {
	return n.conn.Object(nmName, path).GetProperty(name)
}

// ActiveSSID is the SSID of the activated Wi-Fi device ("" when none).
func (n dbusNM) ActiveSSID() (string, error) {
	var devices []dbus.ObjectPath
	if err := n.conn.Object(nmName, nmPath).Call(nmName+".GetDevices", 0).Store(&devices); err != nil {
		return "", err
	}
	for _, d := range devices {
		t, err := n.prop(d, nmName+".Device.DeviceType")
		if err != nil || t.Value() != uint32(2) { // NM_DEVICE_TYPE_WIFI
			continue
		}
		st, err := n.prop(d, nmName+".Device.State")
		if err != nil || st.Value() != uint32(100) { // NM_DEVICE_STATE_ACTIVATED
			continue
		}
		ap, err := n.prop(d, nmName+".Device.Wireless.ActiveAccessPoint")
		if err != nil {
			continue
		}
		path, _ := ap.Value().(dbus.ObjectPath)
		if path == "" || path == "/" {
			continue
		}
		ssid, err := n.prop(path, nmName+".AccessPoint.Ssid")
		if err != nil {
			continue
		}
		b, _ := ssid.Value().([]byte)
		return string(b), nil
	}
	return "", nil
}

// SavedSSIDs lists the Wi-Fi networks NetworkManager has profiles for.
func (n dbusNM) SavedSSIDs() ([]string, error) {
	var conns []dbus.ObjectPath
	if err := n.conn.Object(nmName, nmPath+"/Settings").Call(nmName+".Settings.ListConnections", 0).Store(&conns); err != nil {
		return nil, err
	}
	var out []string
	for _, c := range conns {
		var settings map[string]map[string]dbus.Variant
		if err := n.conn.Object(nmName, c).Call(nmName+".Settings.Connection.GetSettings", 0).Store(&settings); err != nil {
			continue
		}
		if w, ok := settings["802-11-wireless"]; ok {
			if b, ok := w["ssid"].Value().([]byte); ok && len(b) > 0 {
				out = append(out, string(b))
			}
		}
	}
	return out, nil
}
