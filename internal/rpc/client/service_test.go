package client

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/rpc"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// The proxy has exactly app.Service's methods, with identical types: the
// frontend calls it through the same bindings.
func TestService_HasEveryAppMethod(t *testing.T) {
	at, ct := reflect.TypeOf(&app.Service{}), reflect.TypeOf(&Service{})
	require.Equal(t, at.NumMethod(), ct.NumMethod(), "same method set")
	for i := 0; i < at.NumMethod(); i++ {
		am := at.Method(i)
		cm, ok := ct.MethodByName(am.Name)
		require.True(t, ok, am.Name)
		require.Equal(t, am.Type.NumIn(), cm.Type.NumIn(), am.Name)
		for j := 1; j < am.Type.NumIn(); j++ {
			require.Equal(t, am.Type.In(j), cm.Type.In(j), "%s param %d", am.Name, j)
		}
		require.Equal(t, am.Type.NumOut(), cm.Type.NumOut(), am.Name)
		for j := 0; j < am.Type.NumOut(); j++ {
			require.Equal(t, am.Type.Out(j), cm.Type.Out(j), "%s result %d", am.Name, j)
		}
	}
}

func TestService_RegistersEveryBindingID(t *testing.T) {
	src, err := os.ReadFile("service_gen.go")
	require.NoError(t, err)
	re := regexp.MustCompile(`application\.RegisterBindingMethodID\(\(\*Service\)\.(\w+), (\d+)\)`)
	ids := map[string]uint32{}
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		n, err := strconv.ParseUint(m[2], 10, 32)
		require.NoError(t, err)
		ids[m[1]] = uint32(n)
	}
	at := reflect.TypeOf(&app.Service{})
	require.Len(t, ids, at.NumMethod())
	for i := 0; i < at.NumMethod(); i++ {
		name := at.Method(i).Name
		h := fnv.New32a()
		_, _ = h.Write([]byte("github.com/hashcott/ghostline/internal/app.Service." + name))
		require.Equal(t, h.Sum32(), ids[name], name)
	}
}

func TestCodesMatchApp(t *testing.T) {
	require.Equal(t, app.CodeDaemonUnreachable, rpc.CodeUnreachable)
	require.Equal(t, app.CodeDaemonProtocol, rpc.CodeProtocol)
	require.Equal(t, app.CodeNotAuthorized, rpc.CodeNotAuthorized)
	require.Equal(t, app.CodeNoUI, rpc.CodeNoUI)
}

func TestGetSnapshot_DaemonDownIsUnreachable(t *testing.T) {
	s, _ := New(filepath.Join(t.TempDir(), "missing.sock"), quiet(), nil)
	sn := s.GetSnapshot()
	require.Equal(t, app.StatusError, sn.Status)
	require.Equal(t, app.CodeDaemonUnreachable, sn.Error.Code)
	require.NotNil(t, sn.Warnings)
}

type fakeApp struct{}

func (fakeApp) GetSettings() store.Settings { return store.DefaultSettings() }

func startServer(t *testing.T, sock string) *rpc.Server {
	t.Helper()
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := rpc.NewServer(rpc.ServiceHandler(fakeApp{}), "test", func(net.Conn) error { return nil }, quiet())
	go func() { _ = srv.Serve(l) }()
	return srv
}

// Review Focus 3: systemd restarts the daemon while the window is open.
func TestService_ReconnectsAfterDaemonRestart(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	srv := startServer(t, sock)
	s, conn := New(sock, quiet(), nil)
	defer conn.Close()
	events := make(chan string, 8)
	conn.OnEvent(func(name string, _ json.RawMessage) { events <- name })
	require.Equal(t, "vi", s.GetSettings().Language)

	require.NoError(t, srv.Close())
	_ = os.Remove(sock) // Go removes a listener's socket file on Close
	select {            // the drop shows as an unreachable state
	case n := <-events:
		require.Equal(t, app.EventState, n)
	case <-time.After(2 * time.Second):
		t.Fatal("no state event on disconnect")
	}
	srv = startServer(t, sock)
	defer srv.Close()
	require.Equal(t, "vi", s.GetSettings().Language)
	srv.Emit("log", map[string]string{"m": "x"})
	select {
	case n := <-events:
		require.Equal(t, "log", n)
	case <-time.After(2 * time.Second):
		t.Fatal("events did not resume")
	}
}

func TestSetMode_RunsHookAfterCall(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := rpc.NewServer(func(context.Context, string, []json.RawMessage) (json.RawMessage, error) { return nil, nil }, "test", func(net.Conn) error { return nil }, quiet())
	go func() { _ = srv.Serve(l) }()
	defer srv.Close()
	var got string
	s, conn := New(sock, quiet(), func(m string) { got = m })
	defer conn.Close()
	require.NoError(t, s.SetMode("full"))
	require.Equal(t, "full", got)
}

// A daemon that accepts but never answers must not hang the GUI forever.
func TestCall_TimesOutWhenDaemonStalls(t *testing.T) {
	old := callTimeout
	callTimeout = 200 * time.Millisecond
	defer func() { callTimeout = old }()
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	block := make(chan struct{})
	defer close(block)
	srv := rpc.NewServer(func(context.Context, string, []json.RawMessage) (json.RawMessage, error) {
		<-block
		return nil, nil
	}, "test", func(net.Conn) error { return nil }, quiet())
	go func() { _ = srv.Serve(l) }()
	defer srv.Close()
	s, conn := New(sock, quiet(), nil)
	defer conn.Close()
	start := time.Now()
	err = s.Connect()
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), rpc.CodeUnreachable), err.Error())
	require.Less(t, time.Since(start), 3*time.Second)
}

// Only methods that open a GUI dialog take a ctx; they get the long timeout.
func TestDialogMethods_AreTheCtxMethods(t *testing.T) {
	at := reflect.TypeOf(&app.Service{})
	ctxType := reflect.TypeFor[context.Context]()
	got := map[string]bool{}
	for i := 0; i < at.NumMethod(); i++ {
		m := at.Method(i)
		if m.Type.NumIn() > 1 && m.Type.In(1) == ctxType {
			got[m.Name] = true
		}
	}
	require.Equal(t, dialogMethods, got)
}

func TestServiceInstall_DaemonVersion(t *testing.T) {
	old := brand.Version
	t.Cleanup(func() { brand.Version = old })
	cases := []struct {
		daemon, gui string
		outdated    bool
	}{
		{"0.6.0", "0.6.2", true},
		{"0.6.2", "0.6.2", false},
		{"0.6.3", "0.6.2", false}, // never offer a downgrade
		{"0.6.0", "dev", false},
		{"dev", "0.6.2", false},
	}
	for _, c := range cases {
		brand.Version = c.gui
		sock := filepath.Join(t.TempDir(), "ctl.sock")
		l, err := net.Listen("unix", sock)
		require.NoError(t, err)
		srv := rpc.NewServer(rpc.ServiceHandler(fakeApp{}), c.daemon, func(net.Conn) error { return nil }, quiet())
		go func() { _ = srv.Serve(l) }()
		s, conn := New(sock, quiet(), nil)
		got := s.ServiceInstall()
		require.Equal(t, c.daemon, got.ServiceVersion, c)
		require.Equal(t, c.gui, got.AppVersion, c)
		require.Equal(t, c.outdated, got.Outdated, c)
		conn.Close()
		srv.Close()
	}
}

func TestServiceInstall_DaemonDown(t *testing.T) {
	s, conn := New(filepath.Join(t.TempDir(), "none.sock"), quiet(), nil)
	defer conn.Close()
	got := s.ServiceInstall()
	require.Empty(t, got.ServiceVersion)
	require.False(t, got.Outdated)
}
