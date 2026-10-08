package rpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func acceptAll(net.Conn) error { return nil }

// serve starts a server on a fresh socket and returns its path.
func serve(t *testing.T, h Handler, authorize func(net.Conn) error) (*Server, string) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	s := NewServer(h, "test", authorize, quiet())
	go func() { _ = s.Serve(l) }()
	t.Cleanup(func() { _ = s.Close() })
	return s, sock
}

func dial(t *testing.T, sock string) *Client {
	t.Helper()
	c, err := Dial(context.Background(), sock, "test")
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func echo(_ context.Context, method string, args []json.RawMessage) (json.RawMessage, error) {
	if len(args) == 0 {
		return json.RawMessage(`"` + method + `"`), nil
	}
	return args[0], nil
}

func TestCallRoundTrip(t *testing.T) {
	_, sock := serve(t, echo, acceptAll)
	c := dial(t, sock)
	var got map[string]int
	require.NoError(t, c.Call(context.Background(), "Echo", &got, map[string]int{"a": 1}))
	require.Equal(t, map[string]int{"a": 1}, got)
	var name string
	require.NoError(t, c.Call(context.Background(), "Name", &name))
	require.Equal(t, "Name", name)
}

func TestConcurrentCalls(t *testing.T) {
	_, sock := serve(t, echo, acceptAll)
	c := dial(t, sock)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var got int
			require.NoError(t, c.Call(context.Background(), "Echo", &got, i))
			require.Equal(t, i, got)
		}()
	}
	wg.Wait()
}

func TestEventsReachEveryClient(t *testing.T) {
	s, sock := serve(t, echo, acceptAll)
	a, b := dial(t, sock), dial(t, sock)
	got := make(chan string, 2)
	for _, c := range []*Client{a, b} {
		c.OnEvent(func(name string, data json.RawMessage) { got <- name + string(data) })
	}
	// Both clients finished their hello once a call returns.
	require.NoError(t, a.Call(context.Background(), "x", nil))
	require.NoError(t, b.Call(context.Background(), "x", nil))
	s.Emit("state", map[string]int{"a": 1})
	for i := 0; i < 2; i++ {
		select {
		case m := <-got:
			require.Equal(t, `state{"a":1}`, m)
		case <-time.After(2 * time.Second):
			t.Fatal("event not delivered")
		}
	}
}

type codeErr struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params"`
}

func (e *codeErr) Error() string { return e.Code + ": boom" }

// Wails marshals a bound method's error with json.Marshal(&err); a remote
// error must produce the same bytes and the same message.
func TestErrorKeepsMessageAndJSON(t *testing.T) {
	var orig error = &codeErr{Code: "X", Params: map[string]any{"n": 1}}
	_, sock := serve(t, func(context.Context, string, []json.RawMessage) (json.RawMessage, error) { return nil, orig }, acceptAll)
	c := dial(t, sock)
	err := c.Call(context.Background(), "Fail", nil)
	require.Error(t, err)
	require.Equal(t, "X: boom", err.Error())
	want, _ := json.Marshal(&orig)
	got, _ := json.Marshal(&err)
	require.JSONEq(t, string(want), string(got))

	_, sock = serve(t, func(context.Context, string, []json.RawMessage) (json.RawMessage, error) {
		return nil, errors.New("plain")
	}, acceptAll)
	err = dial(t, sock).Call(context.Background(), "Fail", nil)
	got, _ = json.Marshal(&err)
	require.Equal(t, "plain", err.Error())
	require.JSONEq(t, `{}`, string(got))
}

func TestHelloProtocolMismatch(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "ctl.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		_, _ = fmt.Fprintln(c, `{"hello":{"version":"9","protocol":2}}`)
		_, _ = io.Copy(io.Discard, c)
	}()
	_, err = Dial(context.Background(), sock, "test")
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), CodeProtocol), err.Error())
}

func TestAuthorizeRejects(t *testing.T) {
	_, sock := serve(t, echo, func(net.Conn) error { return errors.New("not in the group") })
	_, err := Dial(context.Background(), sock, "test")
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), CodeNotAuthorized), err.Error())
}

func TestLineTooLongClosesConnection(t *testing.T) {
	_, sock := serve(t, echo, acceptAll)
	c, err := net.Dial("unix", sock)
	require.NoError(t, err)
	defer c.Close()
	r := bufio.NewReader(c)
	_, err = r.ReadBytes('\n') // server hello
	require.NoError(t, err)
	_, _ = fmt.Fprintf(c, `{"hello":{"version":"test","protocol":%d}}`+"\n", Protocol)
	go func() { _, _ = c.Write(bytes.Repeat([]byte("a"), MaxLine+1)) }()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.Copy(io.Discard, r)
	require.NoError(t, err, "server closed the connection (EOF) rather than timing out")
}

// Review Focus 4: a backup near backup.MaxSize (8 MiB) travels base64-encoded.
func TestLargeMessageWithinMaxLine(t *testing.T) {
	_, sock := serve(t, echo, acceptAll)
	c := dial(t, sock)
	big := bytes.Repeat([]byte{0xA5}, 8<<20)
	var got []byte
	require.NoError(t, c.Call(context.Background(), "Echo", &got, big))
	require.Equal(t, big, got)
}

// C1: an event handler may call the daemon (the GUI's tray reads settings
// on a "settings" event that arrives before the reply to the save).
func TestEventHandlerMayCall(t *testing.T) {
	var srv *Server
	srv, sock := serve(t, func(_ context.Context, method string, _ []json.RawMessage) (json.RawMessage, error) {
		if method == "Save" {
			srv.Emit("settings", map[string]string{"language": "en"})
		}
		return json.RawMessage(`"ok"`), nil
	}, acceptAll)
	c := dial(t, sock)
	inner := make(chan error, 1)
	c.OnEvent(func(string, json.RawMessage) { inner <- c.Call(context.Background(), "Get", nil) })
	done := make(chan error, 1)
	go func() { done <- c.Call(context.Background(), "Save", nil) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("DEADLOCK: Save never returned")
	}
	select {
	case err := <-inner:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("the handler's own call never returned")
	}
}

// A GUI that stops reading (stuck, or suspended with Ctrl+Z) must not stall
// the daemon: Emit runs under the orchestrator's lock and on the DNS path.
func TestEmitDoesNotBlockOnStuckClient(t *testing.T) {
	s, sock := serve(t, echo, acceptAll)
	c, err := net.Dial("unix", sock)
	require.NoError(t, err)
	defer c.Close()
	r := bufio.NewReader(c)
	_, err = r.ReadBytes('\n') // server hello
	require.NoError(t, err)
	_, _ = fmt.Fprintf(c, `{"hello":{"version":"test","protocol":%d}}`+"\n", Protocol)
	time.Sleep(50 * time.Millisecond) // registered; from now on this client never reads

	payload := strings.Repeat("x", 1024)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20000; i++ { // ~20 MB, far past any socket buffer
			s.Emit("stats", payload)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Emit blocked on a client that does not read")
	}
}

// A result too large for one line is answered with an error, not dropped
// (the caller would otherwise wait for its timeout).
func TestReplyTooLargeIsAnError(t *testing.T) {
	big := json.RawMessage(`"` + strings.Repeat("x", MaxLine) + `"`)
	_, sock := serve(t, func(context.Context, string, []json.RawMessage) (json.RawMessage, error) { return big, nil }, acceptAll)
	c := dial(t, sock)
	done := make(chan error, 1)
	go func() { done <- c.Call(context.Background(), "Big", nil) }()
	select {
	case err := <-done:
		require.EqualError(t, err, "rpc: reply too large")
	case <-time.After(5 * time.Second):
		t.Fatal("the call hung")
	}
}
