package shell

import (
	"errors"
	"fmt"

	"github.com/hashcott/ghostline/internal/brand"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// preflight checks what the window needs before anything else runs.
func preflight() error {
	if !webView2Installed() {
		messageBox(brand.AppName, "Ghostline cần Microsoft Edge WebView2 Runtime.\nGhostline needs the Microsoft Edge WebView2 Runtime.\n\nhttps://go.microsoft.com/fwlink/p/?LinkId=2124703")
		return errors.New("webview2 missing")
	}
	return nil
}

const webView2ClientKey = `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

func webView2Installed() bool {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, path := range []string{webView2ClientKey, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`} {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			v, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && v != "" && v != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

func messageBox(title, text string) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	_, _ = windows.MessageBox(0, m, t, windows.MB_OK|windows.MB_ICONWARNING)
}

func fatalBox(err error) {
	messageBox("Ghostline", fmt.Sprintf("Ghostline không thể khởi động / could not start:\n\n%v", err))
}
