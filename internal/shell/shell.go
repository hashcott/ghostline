// Package shell wires Ghostline together and owns the Wails window and tray.
package shell

import (
	"context"
	"errors"
	"fmt"
	"github.com/hashcott/ghostline/internal/logx"
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
	// An unrecovered panic in any goroutine kills the process; a GUI exe has
	// no stderr, so send the crash report (with stacks) to a file.
	if err := logx.CrashOutput(p.Paths.LogDir, "ghostline-crash"); err != nil {
		log.Warn("shell: crash output setup failed", "dir", p.Paths.LogDir, "err", err)
	}
	log.Info("start", append([]any{"version", brand.Version, "portable", p.Paths.Portable, "mode", o.Mode.Kind.String()}, envAttrs()...)...)

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
		RunInstaller: installerRunner(o.Executable, p.Paths.Portable, func() {
			if wapp != nil {
				wapp.Quit()
			}
		}),
	})
	if err != nil {
		fatalBox(err)
		return err
	}
	ui.b = c.Svc
	ui.selfUpdate = c.Svc.AppInfo().SelfUpdate
	ui.saveFullWindow = func(w, h int) { // straight to settings.json, as before validation existed
		s := c.Settings.Get()
		s.FullWindow.Width, s.FullWindow.Height = w, h
		if err := c.Settings.Save(s); err != nil {
			log.Warn("ui: saving the window size failed", "err", err)
		}
	}

	opts := application.Options{
		Name:        brand.AppName,
		Description: "Secure DNS client",
		Services:    []application.Service{application.NewService(c.Svc)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(o.Assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: brand.SingleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				log.Info("shell: second instance launched; showing the window")
				ui.show()
			},
		},
		OnShutdown: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := c.Shutdown(ctx); err != nil {
				log.Warn("shell: disconnect at shutdown failed", "err", err)
			}
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

	// Opened by the installer of an update started while connected, or
	// started with Windows with auto-connect on.
	afterUpdate := c.ReconnectAfterUpdate()
	if afterUpdate || (o.Mode.Kind == cli.KindAutostart && c.Settings.Get().AutoConnect) {
		go func() {
			if err := c.Orch.Connect(context.Background()); err != nil {
				log.Warn("shell: auto-connect failed", "afterUpdate", afterUpdate, "err", err)
			}
		}()
	}
	if err := wapp.Run(); err != nil {
		log.Error("shell: wails run failed", "err", err)
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
