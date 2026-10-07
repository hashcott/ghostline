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
