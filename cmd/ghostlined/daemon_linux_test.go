package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/backup"
	"github.com/hashcott/ghostline/internal/rpc/client"
	"github.com/stretchr/testify/require"
)

func TestDaemon_EndToEnd(t *testing.T) {
	if os.Geteuid() == 0 {
		// As root, Connect would really change this machine's DNS; the
		// root integration tests cover that path on purpose.
		t.Skip("runs only as a normal user")
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "ctl.sock")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runDaemon(ctx, Args{Command: "daemon", DataDir: filepath.Join(dir, "d"), Socket: sock, AllowUID: os.Getuid()})
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(sock); return err == nil }, 5*time.Second, 20*time.Millisecond)

	s, conn := client.New(sock, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	defer conn.Close()
	events := make(chan string, 64)
	conn.OnEvent(func(name string, _ json.RawMessage) { events <- name })
	uiKinds := make(chan []json.RawMessage, 1)
	conn.OnUI(func(_ context.Context, kind string, args []json.RawMessage) (any, error) {
		require.Equal(t, "saveFile", kind)
		uiKinds <- args
		return nil, nil
	})

	st := s.GetSettings()
	require.Equal(t, "vi", st.Language)
	st.Language = "en"
	require.NoError(t, s.SaveSettings(st))
	require.Eventually(t, func() bool {
		for {
			select {
			case n := <-events:
				if n == "settings" {
					return true
				}
			default:
				return false
			}
		}
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, app.StatusDisconnected, s.GetSnapshot().Status)
	require.Error(t, s.Connect(), "no system support yet: connecting fails before any change")

	require.NoError(t, s.ExportSettings(context.Background(), backup.AllSections))
	select {
	case args := <-uiKinds:
		var name string
		require.NoError(t, json.Unmarshal(args[0], &name))
		require.Regexp(t, `\.ghostline\.json$`, name)
	case <-time.After(2 * time.Second):
		t.Fatal("no saveFile reverse call")
	}

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(12 * time.Second):
		t.Fatal("daemon did not stop")
	}
	_, err := os.Stat(sock)
	require.True(t, os.IsNotExist(err), "socket removed on shutdown")
}

// A second daemon on the same directories stops before doing anything:
// no startup restore, no log rotation.
func TestRunDaemon_SecondInstanceDoesNothing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("runs only as a normal user")
	}
	dir := t.TempDir()
	data := filepath.Join(dir, "d")
	sock := filepath.Join(dir, "ctl.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runDaemon(ctx, Args{Command: "daemon", DataDir: data, Socket: sock, AllowUID: os.Getuid()})
	}()
	require.Eventually(t, func() bool { _, err := os.Stat(sock); return err == nil }, 5*time.Second, 20*time.Millisecond)

	logFile := filepath.Join(data, "log", "ghostline.log")
	before, err := os.Stat(logFile)
	require.NoError(t, err)
	err = runDaemon(context.Background(), Args{Command: "daemon", DataDir: data, Socket: filepath.Join(dir, "other.sock"), AllowUID: os.Getuid()})
	require.ErrorContains(t, err, "another ghostlined is running")
	after, err := os.Stat(logFile)
	require.NoError(t, err)
	require.Equal(t, before.Size(), after.Size(), "the second instance wrote nothing")
	_, err = os.Stat(filepath.Join(dir, "other.sock"))
	require.True(t, os.IsNotExist(err))

	cancel()
	require.NoError(t, <-done)
}
