package shell

import (
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/stretchr/testify/require"
)

func TestTrayText_FollowsLanguage(t *testing.T) { // review I10
	vi, en := trayText("vi"), trayText("en")
	require.Equal(t, "Thoát", vi.quit)
	require.Equal(t, "Quit", en.quit)
	require.Equal(t, "Connect", en.connect)
	require.Equal(t, "Disconnect", en.disconnect)
	require.Equal(t, "Protected", en.status[app.StatusProtected])
	require.Equal(t, "Đã bảo vệ", vi.status[app.StatusProtected])
	require.Equal(t, en, trayText("fr"), "unknown languages fall back to English")
	for _, s := range []app.Status{app.StatusDisconnected, app.StatusConnecting, app.StatusProtected, app.StatusDegraded, app.StatusDisconnecting, app.StatusError} {
		require.NotEmpty(t, vi.status[s])
		require.NotEmpty(t, en.status[s])
	}
}

func TestTrayText_UpdateLabel(t *testing.T) {
	require.Equal(t, "Có bản mới v0.1.1 ↗", trayText("vi").updateLabel("v0.1.1"))
	require.Equal(t, "New version v0.1.1 ↗", trayText("en").updateLabel("v0.1.1"))
	require.Equal(t, "Cập nhật lên v0.1.1…", trayText("vi").installLabel("v0.1.1"))
	require.Equal(t, "Update to v0.1.1…", trayText("en").installLabel("v0.1.1"))
}

func TestTrayText_ProxyItem(t *testing.T) {
	require.Equal(t, "Proxy: bật", trayText("vi").proxyLabel(true))
	require.Equal(t, "Proxy: tắt", trayText("vi").proxyLabel(false))
	require.Equal(t, "Proxy: on", trayText("en").proxyLabel(true))
	require.Equal(t, "Proxy: off", trayText("en").proxyLabel(false))
}

func TestTrayText_CheckUpdate(t *testing.T) {
	require.Equal(t, "Kiểm tra cập nhật", trayText("vi").checkUpdate)
	require.Equal(t, "Check for updates", trayText("en").checkUpdate)
	require.Equal(t, "Đã là bản mới nhất", trayText("vi").upToDate)
	require.Equal(t, "Up to date", trayText("en").upToDate)
	require.Equal(t, "Không kiểm tra được cập nhật", trayText("vi").checkFailed)
	require.Equal(t, "Could not check for updates", trayText("en").checkFailed)
}

func TestTrayText_FakeSNI(t *testing.T) {
	require.Equal(t, "Ghostline · Đã bảo vệ · Fake SNI: bật", trayText("vi").tooltip(app.StatusProtected, "", true))
	require.Equal(t, "Ghostline · Protected · Fake SNI: on", trayText("en").tooltip(app.StatusProtected, "", true))
	require.Equal(t, "Ghostline · Protected · New version v1 ↗", trayText("en").tooltip(app.StatusProtected, "v1", false))
}

func TestTrayText_DisconnectAsk(t *testing.T) {
	if got := trayText("vi").disconnectAsk(2); !strings.Contains(got, "2 thiết bị") || !strings.Contains(got, "mất mạng") {
		t.Errorf("vi = %q", got)
	}
	if got := trayText("en").disconnectAsk(1); !strings.Contains(got, ": 1.") || !strings.Contains(got, "lose the internet") {
		t.Errorf("en = %q", got)
	}
}
