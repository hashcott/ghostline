//go:build linux

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/core"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/rpc"
	"github.com/hashcott/ghostline/internal/store"
)

// depsFor picks the daemon's layout: the system one, or --data-dir.
func depsFor(a Args) (platform.Deps, error) {
	if a.DataDir != "" {
		return platform.NewDev(a.DataDir)
	}
	exe, err := os.Executable()
	if err != nil {
		return platform.Deps{}, err
	}
	return platform.New(exe)
}

// runDaemon serves core over the control socket until ctx is done, then
// disconnects cleanly so the system is as it was.
func runDaemon(ctx context.Context, a Args) error {
	p, err := depsFor(a)
	if err != nil {
		return err
	}
	socket := a.Socket
	if socket == "" {
		socket = p.Socket
	}
	if err := os.MkdirAll(filepath.Dir(p.Socket), 0o755); err != nil { // the state lock lives there too
		return err
	}
	log, logw, err := core.OpenLog(p.Paths)
	if err != nil {
		return err
	}
	defer logw.Close()
	log.Info("start", "version", brand.Version, "socket", socket)

	var extra []int
	if a.AllowUID >= 0 {
		extra = []int{a.AllowUID}
	}
	var handler rpc.Handler // set once core exists; nothing is served before
	srv := rpc.NewServer(func(ctx context.Context, m string, args []json.RawMessage) (json.RawMessage, error) {
		return handler(ctx, m, args)
	}, brand.Version, rpc.AllowGroups(rpc.DefaultGroups, extra...), log)

	c, err := core.New(core.Options{
		Platform: p, Log: log, Emitter: srv,
		// The GUI relabels its tray from this; it is not a frontend event.
		OnSettingsChanged: func(_, n store.Settings) { srv.Emit("settings", n) },
		OpenFile: func(ctx context.Context, title string) (string, []byte, error) {
			var r struct {
				Name string `json:"name"`
				Data []byte `json:"data"`
			}
			if err := rpc.CallUI(ctx, "openFile", &r, title); err != nil {
				return "", nil, err
			}
			return r.Name, r.Data, nil
		},
		SaveFile: func(ctx context.Context, name string, data []byte) error {
			return rpc.CallUI(ctx, "saveFile", nil, name, data)
		},
	})
	if err != nil {
		return err
	}
	handler = rpc.ServiceHandler(c.Svc)

	l, err := listen(socket)
	if err != nil {
		return err
	}
	runCtx, stopRun := context.WithCancel(context.Background())
	wait := c.Start(runCtx)
	defer func() { stopRun(); wait() }() // every loop is done before we return
	go func() { _ = srv.Serve(l) }()
	if s := c.Settings.Get(); s.StartWithWindows && s.AutoConnect {
		go func() { _ = c.Orch.Connect(context.Background()) }()
	}

	<-ctx.Done()
	log.Info("stopping")
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Shutdown(sctx); err != nil {
		log.Error("shutdown", "err", err)
	}
	_ = srv.Close()
	_ = os.Remove(socket)
	return nil
}
