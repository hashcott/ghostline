// Package shell wires Ghostline together and owns the Wails window and tray.
package shell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/core"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Options carries what main provides.
type Options struct {
	Mode       cli.Mode
	Assets     fs.FS
	Executable string
	// Platform is this OS's wiring (platform.New).
	Platform platform.Deps
}

// Fatal reports an error that stops Ghostline before its window exists.
func Fatal(err error) { fatalBox(err) }

// Run starts the UI process.
func Run(o Options) error {
	if o.Platform.UsesDaemon {
		return runClient(o)
	}
	if err := preflight(); err != nil {
		return err
	}
	p := o.Platform
	log, logw, err := core.OpenLog(p.Paths)
	if err != nil {
		fatalBox(err)
		return err
	}
	defer logw.Close()
	slog.SetDefault(log)
	log.Info("start", "version", brand.Version, "portable", p.Paths.Portable, "mode", o.Mode.Kind)

	var wapp *application.App
	em := &emitter{}
	ui := &ui{log: log}
	c, err := core.New(core.Options{
		Platform: p, Log: log, Emitter: em,
		SetMode: ui.setMode,
		OnSettingsChanged: func(old, n store.Settings) {
			if old.Language != n.Language {
				ui.onLanguage()
			}
			if old.Proxy.Enabled != n.Proxy.Enabled {
				ui.onLanguage() // relabels the tray's proxy item
			}
		},
		OnUpdate: func(tag, url string) { ui.onUpdate(tag, url) },
		OpenFile: func(_ context.Context, title string) (string, []byte, error) {
			if wapp == nil {
				return "", nil, errors.New("no window")
			}
			path, err := wapp.Dialog.OpenFile().SetTitle(title).AddFilter("Ghostline backup (*.json)", "*.json").PromptForSingleSelection()
			if err != nil || path == "" {
				return "", nil, err // cancelled
			}
			data, err := readLimited(path, backup.MaxSize+1)
			if err != nil {
				return "", nil, err
			}
			return filepath.Base(path), data, nil
		},
		SaveFile: func(_ context.Context, name string, data []byte) error {
			if wapp == nil {
				return errors.New("no window")
			}
			path, err := wapp.Dialog.SaveFile().SetFilename(name).PromptForSingleSelection()
			if err != nil || path == "" {
				return err // cancelled: nothing to save
			}
			return os.WriteFile(path, data, 0o644)
		},
	})
	if err != nil {
		fatalBox(err)
		return err
	}
	ui.b = c.Svc

	opts := application.Options{
		Name:        brand.AppName,
		Description: "Secure DNS client",
		Services:    []application.Service{application.NewService(c.Svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(o.Assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               brand.SingleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { ui.show() },
		},
		OnShutdown: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = c.Shutdown(ctx)
		},
	}
	applyPlatformOptions(&opts, c.Orch)
	wapp = application.New(opts)
	em.app = wapp
	ui.app = wapp
	if o.Mode.Kind != cli.KindAutostart {
		// Started with Windows: stay in the tray without a window, so no
		// WebView2 runs until the user opens Ghostline.
		ui.createWindow()
	}
	ui.createTray()
	em.onState = ui.onState

	ctx, cancel := context.WithCancel(context.Background())
	wait := c.Start(ctx)
	defer func() { cancel(); wait() }()

	if o.Mode.Kind == cli.KindAutostart && c.Settings.Get().AutoConnect {
		go func() { _ = c.Orch.Connect(context.Background()) }()
	}
	if err := wapp.Run(); err != nil {
		fatalBox(err)
		return fmt.Errorf("shell: %w", err)
	}
	return nil
}

// readLimited reads at most limit bytes of path: a file the user picked
// for import can be anything.
func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}
