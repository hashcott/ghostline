package shell

import (
	"image/color"
	"log/slog"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/icon"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// emitter forwards events to Wails and lets the shell watch state changes.
type emitter struct {
	app     *application.App
	onState func(app.Snapshot)
}

func (e *emitter) Emit(name string, data any) {
	if e.app != nil {
		e.app.Event.Emit(name, data)
	}
	if name == app.EventState && e.onState != nil {
		if s, ok := data.(app.Snapshot); ok {
			e.onState(s)
		}
	}
}

const (
	simpleW, simpleH   = 380, 580
	minFullW, minFullH = 900, 600
)

var statusColour = map[app.Status]color.RGBA{
	app.StatusDisconnected:  {0x3a, 0x4a, 0x44, 0xff},
	app.StatusConnecting:    {0x00, 0xd0, 0xff, 0xff},
	app.StatusDisconnecting: {0x00, 0xd0, 0xff, 0xff},
	app.StatusProtected:     {0x00, 0xff, 0xa3, 0xff},
	app.StatusDegraded:      {0xff, 0xb0, 0x20, 0xff},
	app.StatusError:         {0xff, 0x4d, 0x6d, 0xff},
}

type ui struct {
	app *application.App
	b   trayBackend
	log *slog.Logger
	// saveFullWindow remembers the full interface's size. It runs on the
	// main thread, so it must not wait for the daemon.
	saveFullWindow func(w, h int)

	mu sync.Mutex
	// win is the open window, nil while Ghostline sits in the tray. Only
	// touched on the main thread (or before the app runs).
	win         *application.WebviewWindow
	tray        *application.SystemTray
	connItem    *application.MenuItem
	dpiItem     *application.MenuItem
	proxyItem   *application.MenuItem
	checkItem   *application.MenuItem
	openItem    *application.MenuItem
	quitItem    *application.MenuItem
	updItem     *application.MenuItem
	lastState   app.Status
	lastFakeSNI bool
	updTag      string // newer release, "" if none
	updURL      string
}

// createWindow opens the main window. Closing it to the tray destroys it
// (and its WebView2 processes, ~150 MB) rather than hiding it; show()
// creates a new one.
func (u *ui) createWindow() {
	// The initial size must match the saved mode: resizing a window that
	// has not been shown yet is ignored.
	s := u.b.GetSettings()
	opts := application.WebviewWindowOptions{
		Name:                "main",
		Title:               brand.AppName,
		Width:               simpleW,
		Height:              simpleH,
		Frameless:           true,
		DisableResize:       true,
		MaximiseButtonState: application.ButtonDisabled,
		BackgroundColour:    application.NewRGB(5, 7, 10),
		URL:                 "/",
	}
	if s.Mode == store.ModeFull {
		opts.Width, opts.Height = max(s.FullWindow.Width, minFullW), max(s.FullWindow.Height, minFullH)
		opts.MinWidth, opts.MinHeight = minFullW, minFullH
		opts.DisableResize = false
	}
	w := u.app.Window.NewWithOptions(opts)
	u.win = w
	w.RegisterHook(events.Common.WindowClosing, func(*application.WindowEvent) {
		if u.b.GetSettings().CloseToTray {
			if u.win == w {
				u.win = nil // let it close; the tray stays
			}
			return
		}
		u.app.Quit()
	})
}

// show brings the window up, creating it if it was closed to the tray.
func (u *ui) show() {
	application.InvokeSync(func() {
		if u.win == nil {
			u.createWindow()
		}
		u.win.Show()
		u.win.Focus()
	})
}

// setMode resizes the window while keeping its centre in place.
func (u *ui) setMode(mode string) {
	application.InvokeSync(func() { u.resize(mode) })
}

func (u *ui) resize(mode string) {
	if u.win == nil {
		return
	}
	x, y := u.win.Position()
	w0, h0 := u.win.Size()
	cx, cy := x+w0/2, y+h0/2
	s := u.b.GetSettings()
	w, h := simpleW, simpleH
	if mode == store.ModeFull {
		w, h = max(s.FullWindow.Width, minFullW), max(s.FullWindow.Height, minFullH)
		u.win.SetResizable(true)
		u.win.SetMinSize(minFullW, minFullH)
	} else {
		if w0 >= minFullW { // remember the full-interface size
			u.saveFullWindow(w0, h0)
		}
		u.win.SetMinSize(simpleW, simpleH)
		u.win.SetResizable(false)
	}
	u.win.SetSize(w, h)
	u.win.SetPosition(cx-w/2, cy-h/2)
}

func (u *ui) createTray() {
	u.tray = u.app.SystemTray.New()
	u.tray.SetIcon(icon.Ring(statusColour[app.StatusDisconnected], trayIconSize()))
	tt := trayText(u.b.GetSettings().Language)
	u.tray.SetTooltip(brand.AppName + " · " + tt.status[app.StatusDisconnected])
	menu := application.NewMenu()
	u.updItem = menu.Add("").SetHidden(true).OnClick(func(*application.Context) {
		u.mu.Lock()
		url := u.updURL
		u.mu.Unlock()
		if url != "" {
			if err := u.app.Browser.OpenURL(url); err != nil {
				u.log.Warn("tray: opening the release page failed", "err", err)
			}
		}
	})
	u.connItem = menu.Add(tt.connect).OnClick(func(*application.Context) {
		go func() {
			if err := trayToggle(u.b, func(n int) bool { return u.confirmDisconnect(tt, n) }); err != nil {
				u.log.Warn("tray: connect or disconnect failed", "err", err)
			}
		}()
	})
	u.dpiItem = menu.AddCheckbox(tt.dpi, u.b.GetSettings().DPI.Enabled)
	u.dpiItem.OnClick(func(c *application.Context) {
		on := c.IsChecked()
		go func() {
			if err := u.b.SetDPIEnabled(on); err != nil {
				u.log.Warn("tray: switching DPI bypass failed", "on", on, "err", err)
				u.dpiItem.SetChecked(!on)
			}
		}()
	})
	u.proxyItem = menu.Add(tt.proxyLabel(u.b.GetSettings().Proxy.Enabled)).OnClick(func(*application.Context) {
		go func() {
			on := !u.b.GetSettings().Proxy.Enabled
			if err := u.b.SetProxyEnabled(on); err != nil {
				u.log.Warn("tray: switching the proxy failed", "on", on, "err", err)
			}
			u.onLanguage()
		}()
	})
	menu.AddSeparator()
	u.checkItem = menu.Add(tt.checkUpdate).OnClick(func(*application.Context) { go u.checkUpdate() })
	u.openItem = menu.Add(tt.open).OnClick(func(*application.Context) { u.show() })
	menu.Add("Ghostline " + brand.Version).SetEnabled(false)
	menu.AddSeparator()
	u.quitItem = menu.Add(tt.quit).OnClick(func(*application.Context) { u.app.Quit() })
	u.tray.SetMenu(menu)
	u.tray.OnClick(u.show)
	u.tray.OnRightClick(u.tray.OpenMenu) // works around tray menu issue #6161
}

func (u *ui) onState(s app.Snapshot) {
	u.mu.Lock()
	changed := s.Status != u.lastState || s.FakeSNI.Active != u.lastFakeSNI
	u.lastState = s.Status
	u.lastFakeSNI = s.FakeSNI.Active
	u.mu.Unlock()
	if !changed || u.tray == nil {
		return
	}
	u.tray.SetIcon(icon.Ring(statusColour[s.Status], trayIconSize()))
	u.relabel(s.Status)
	u.dpiItem.SetChecked(s.DPI.Enabled)
}

// relabel applies the current language to the tray (also called when the
// language setting changes).
func (u *ui) relabel(status app.Status) {
	if u.tray == nil {
		return
	}
	tt := trayText(u.b.GetSettings().Language)
	u.mu.Lock()
	tag := u.updTag
	fakeSNI := u.lastFakeSNI
	u.mu.Unlock()
	if tag != "" {
		u.updItem.SetLabel(tt.updateLabel(tag)).SetHidden(false)
	}
	u.tray.SetTooltip(tt.tooltip(status, tag, fakeSNI))
	if status == app.StatusProtected || status == app.StatusDegraded {
		u.connItem.SetLabel(tt.disconnect)
	} else {
		u.connItem.SetLabel(tt.connect)
	}
	u.dpiItem.SetLabel(tt.dpi)
	u.proxyItem.SetLabel(tt.proxyLabel(u.b.GetSettings().Proxy.Enabled))
	u.checkItem.SetLabel(tt.checkUpdate)
	u.openItem.SetLabel(tt.open)
	u.quitItem.SetLabel(tt.quit)
}

// onLanguage re-labels the tray after a language change.
func (u *ui) onLanguage() {
	u.mu.Lock()
	st := u.lastState
	u.mu.Unlock()
	if st == "" {
		st = app.StatusDisconnected
	}
	u.relabel(st)
}

// onUpdate shows a newer release in the tray menu and tooltip.
func (u *ui) onUpdate(tag, url string) {
	u.mu.Lock()
	u.updTag, u.updURL = tag, url
	st := u.lastState
	u.mu.Unlock()
	if st == "" {
		st = app.StatusDisconnected
	}
	u.relabel(st)
}

// checkUpdate is the tray's "Check for updates": a newer release shows up
// through onUpdate (menu item + tooltip); otherwise the tooltip says so.
func (u *ui) checkUpdate() {
	if u.b == nil || u.tray == nil {
		return
	}
	r, err := u.b.CheckUpdateNow()
	if err == nil && r.Newer {
		return
	}
	tt := trayText(u.b.GetSettings().Language)
	msg := tt.upToDate + " (" + brand.Version + ")"
	if err != nil {
		u.log.Warn("tray: update check failed", "err", err)
		msg = tt.checkFailed
	}
	u.tray.SetTooltip(brand.AppName + " · " + msg)
	time.AfterFunc(10*time.Second, u.onLanguage) // back to the status tooltip
}

// confirmDisconnect asks before a tray disconnect while n LAN devices use
// this PC's DNS: they lose the internet with it. Shutdown and Quit do not
// ask.
func (u *ui) confirmDisconnect(tt trayStrings, n int) bool {
	if u.app == nil {
		return true
	}
	ok := false
	d := u.app.Dialog.Question().SetTitle(brand.AppName).SetMessage(tt.disconnectAsk(n))
	// Windows shows a system Yes/No box and matches callbacks by these
	// labels; the buttons themselves are in the OS language.
	d.AddButton("Yes").OnClick(func() { ok = true })
	no := d.AddButton("No")
	d.SetDefaultButton(no).SetCancelButton(no).Show()
	return ok
}
