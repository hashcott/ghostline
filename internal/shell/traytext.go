package shell

import (
	"fmt"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
)

// trayStrings is the tray menu copy for one language (the tray lives in Go,
// outside the React i18n).
type trayStrings struct {
	connect, disconnect, dpi, open, quit, update string // update: "%s" is the tag
	install                                      string // "%s" is the tag; builds that update themselves
	proxyOn, proxyOff                            string
	fakeSNIOn                                    string
	checkUpdate, upToDate, checkFailed           string
	disconnectQ                                  string // "%d" is the LAN device count
	status                                       map[app.Status]string
}

var trayLangs = map[string]trayStrings{
	"vi": {
		connect: "Kết nối", disconnect: "Ngắt kết nối", dpi: "Vượt DPI", open: "Mở Ghostline", quit: "Thoát",
		update: "Có bản mới %s ↗", install: "Cập nhật lên %s…", proxyOn: "Proxy: bật", proxyOff: "Proxy: tắt", fakeSNIOn: "Fake SNI: bật",
		checkUpdate: "Kiểm tra cập nhật", upToDate: "Đã là bản mới nhất", checkFailed: "Không kiểm tra được cập nhật",
		disconnectQ: "%d thiết bị trong mạng đang dùng DNS của máy này và sẽ mất mạng. Vẫn ngắt kết nối?",
		status: map[app.Status]string{
			app.StatusDisconnected: "Chưa bảo vệ", app.StatusConnecting: "Đang kết nối", app.StatusProtected: "Đã bảo vệ",
			app.StatusDegraded: "Suy giảm", app.StatusDisconnecting: "Đang ngắt", app.StatusError: "Lỗi",
		},
	},
	"en": {
		connect: "Connect", disconnect: "Disconnect", dpi: "DPI bypass", open: "Open Ghostline", quit: "Quit",
		update: "New version %s ↗", install: "Update to %s…", proxyOn: "Proxy: on", proxyOff: "Proxy: off", fakeSNIOn: "Fake SNI: on",
		checkUpdate: "Check for updates", upToDate: "Up to date", checkFailed: "Could not check for updates",
		disconnectQ: "Devices on your network using this PC's DNS: %d. They will lose the internet. Disconnect anyway?",
		status: map[app.Status]string{
			app.StatusDisconnected: "Unprotected", app.StatusConnecting: "Connecting", app.StatusProtected: "Protected",
			app.StatusDegraded: "Degraded", app.StatusDisconnecting: "Disconnecting", app.StatusError: "Error",
		},
	},
}

func trayText(lang string) trayStrings {
	if t, ok := trayLangs[lang]; ok {
		return t
	}
	return trayLangs["en"]
}

func (t trayStrings) updateLabel(tag string) string { return fmt.Sprintf(t.update, tag) }

// installLabel names the menu item that opens the window's update button.
func (t trayStrings) installLabel(tag string) string { return fmt.Sprintf(t.install, tag) }

// disconnectAsk asks before a disconnect that cuts n LAN devices off.
func (t trayStrings) disconnectAsk(n int) string { return fmt.Sprintf(t.disconnectQ, n) }

// tooltip is the tray tooltip: status, a pending update, and Fake SNI
// while it decrypts traffic (spec 2B 9.2).
func (t trayStrings) tooltip(status app.Status, updateTag string, fakeSNI bool) string {
	tip := brand.AppName + " · " + t.status[status]
	if updateTag != "" {
		tip += " · " + t.updateLabel(updateTag)
	}
	if fakeSNI {
		tip += " · " + t.fakeSNIOn
	}
	return tip
}

func (t trayStrings) proxyLabel(on bool) string {
	if on {
		return t.proxyOn
	}
	return t.proxyOff
}
