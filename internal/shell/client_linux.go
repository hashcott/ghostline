package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/logx"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/rpc/client"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// runClient is the Linux GUI: a window and tray over ghostlined. The
// frontend talks to client.Service, which forwards every call to the
// daemon; the daemon's events come back and are re-emitted to it.
func runClient(o Options) error {
	log, logw := clientLog()
	defer logw.Close()
	slog.SetDefault(log)
	log.Info("start", "version", brand.Version, "socket", platform.ClientSocket())

	ui := &ui{log: log}
	svc, conn := client.New(platform.ClientSocket(), log, ui.setMode)
	defer conn.Close()
	settings := newSettingsCache(svc)
	ui.b = settings
	autostart := func(s store.Settings) {
		if err := applyAutostart(s, os.Getenv, os.Executable); err != nil {
			log.Warn("autostart entry", "err", err)
		}
	}
	autostart(settings.GetSettings())
	ui.saveFullWindow = func(w, h int) {
		go func() {
			s := settings.GetSettings()
			s.FullWindow.Width, s.FullWindow.Height = w, h
			_ = settings.SaveSettings(s)
		}()
	}

	var wapp *application.App
	em := &emitter{}
	conn.OnEvent(func(name string, data json.RawMessage) {
		switch name {
		case "settings": // language, proxy or start with the system changed
			var n store.Settings
			if json.Unmarshal(data, &n) == nil {
				settings.set(n)
				autostart(n)
			}
			ui.onLanguage()
			return
		case app.EventUpdate:
			var u app.UpdateInfo
			if json.Unmarshal(data, &u) == nil {
				ui.onUpdate(u.Tag, u.URL)
			}
		}
		if v, ok := decodeEvent(name, data); ok {
			em.Emit(name, v)
		}
	})
	saveFile := saveFileHandler(func(name string) (string, error) {
		return wapp.Dialog.SaveFile().SetFilename(name).PromptForSingleSelection()
	})
	openFile := openFileHandler(func(title string) (string, error) {
		return wapp.Dialog.OpenFile().SetTitle(title).AddFilter("Ghostline backup (*.json)", "*.json").PromptForSingleSelection()
	})
	conn.OnUI(func(_ context.Context, kind string, args []json.RawMessage) (any, error) {
		switch kind {
		case "saveFile":
			return saveFile(args)
		case "openFile":
			return openFile(args)
		}
		return nil, fmt.Errorf("unknown ui call %q", kind)
	})

	opts := application.Options{
		Name:        brand.AppName,
		Description: "Secure DNS client",
		Services:    []application.Service{application.NewService(svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(o.Assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               brand.SingleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { ui.show() },
		},
	}
	applyPlatformOptions(&opts, nil)
	wapp = application.New(opts)
	em.app = wapp
	ui.app = wapp
	if o.Mode.Kind != cli.KindAutostart {
		ui.createWindow()
	}
	ui.createTray()
	em.onState = ui.onState
	if err := wapp.Run(); err != nil {
		fatalBox(err)
		return fmt.Errorf("shell: %w", err)
	}
	return nil
}

// clientLog writes the GUI's log under ~/.config/ghostline/logs; without
// a config directory it goes to stderr.
func clientLog() (*slog.Logger, io.Closer) {
	if dir, err := os.UserConfigDir(); err == nil {
		if w, err := logx.NewRotating(filepath.Join(dir, "ghostline", "logs"), "ghostline-gui", 5<<20, 3); err == nil {
			return slog.New(slog.NewTextHandler(w, nil)), w
		}
	}
	return slog.New(slog.NewTextHandler(os.Stderr, nil)), io.NopCloser(nil)
}
