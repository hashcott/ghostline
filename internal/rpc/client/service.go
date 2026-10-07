package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/rpc"
)

// dialTimeout bounds connecting to the daemon.
const dialTimeout = 3 * time.Second

// callTimeout bounds a call: Connect may scan servers for minutes, but a
// daemon that stops answering must not hang the GUI forever. Methods that
// open a file dialog in the GUI get rpc.UITimeout more.
var callTimeout = 10 * time.Minute

var dialogMethods = map[string]bool{"ExportSettings": true, "PreviewImport": true, "ExportAdvancedCSV": true, "SaveDeviceFiles": true}

// Conn manages the connection behind a Service: it dials on first use and
// again after the daemon restarts. Its handlers live here, not on Service,
// so Service's methods are app.Service's and nothing else.
type Conn struct {
	socket string
	log    *slog.Logger

	mu      sync.Mutex
	c       *rpc.Client
	onEvent func(name string, data json.RawMessage)
	onUI    func(ctx context.Context, kind string, args []json.RawMessage) (any, error)
}

// Service is app.Service over the control socket (methods in service_gen.go).
type Service struct {
	conn   *Conn
	log    *slog.Logger
	onMode func(mode string)
}

// New returns the proxy and its connection. onMode runs after a successful
// SetMode (the window resizes); nil for none.
func New(socket string, log *slog.Logger, onMode func(mode string)) (*Service, *Conn) {
	c := &Conn{socket: socket, log: log}
	return &Service{conn: c, log: log, onMode: onMode}, c
}

// OnEvent sets the handler for the daemon's events. When the connection
// drops it also gets a "state" event with an unreachable snapshot.
func (c *Conn) OnEvent(fn func(name string, data json.RawMessage)) {
	c.mu.Lock()
	c.onEvent = fn
	c.mu.Unlock()
}

// OnUI sets the handler for the daemon's reverse ui calls.
func (c *Conn) OnUI(fn func(ctx context.Context, kind string, args []json.RawMessage) (any, error)) {
	c.mu.Lock()
	c.onUI = fn
	if c.c != nil {
		c.c.OnUI(fn)
	}
	c.mu.Unlock()
}

// Close drops the connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	cl := c.c
	c.c = nil
	c.onEvent = nil
	c.mu.Unlock()
	if cl != nil {
		return cl.Close()
	}
	return nil
}

// client returns a live connection, dialling when there is none.
func (c *Conn) client() (*rpc.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.c != nil {
		select {
		case <-c.c.Done():
			c.c = nil
		default:
			return c.c, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	cl, err := rpc.Dial(ctx, c.socket, brand.Version)
	if err != nil {
		return nil, err
	}
	cl.OnEvent(func(name string, data json.RawMessage) { c.emit(name, data) })
	cl.OnUI(c.onUI)
	c.c = cl
	go func() {
		<-cl.Done()
		c.mu.Lock()
		current := c.c == cl
		c.mu.Unlock()
		if current {
			c.emit(app.EventState, unreachable(rpc.CodeUnreachable))
		}
	}()
	return cl, nil
}

func (c *Conn) emit(name string, data any) {
	c.mu.Lock()
	fn := c.onEvent
	c.mu.Unlock()
	if fn == nil {
		return
	}
	raw, ok := data.(json.RawMessage)
	if !ok {
		b, err := json.Marshal(data)
		if err != nil {
			return
		}
		raw = b
	}
	fn(name, raw)
}

func (s *Service) call(ctx context.Context, method string, result any, args ...any) error {
	cl, err := s.conn.client()
	if err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		d := callTimeout
		if dialogMethods[method] {
			d += rpc.UITimeout
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	err = cl.Call(ctx, method, result, args...)
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s: %s got no answer: %w", rpc.CodeUnreachable, method, err)
	}
	return err
}

// callTuple decodes a method's several results (sent as a JSON array).
func (s *Service) callTuple(ctx context.Context, method string, results []any, args ...any) error {
	var raw []json.RawMessage
	if err := s.call(ctx, method, &raw, args...); err != nil {
		return err
	}
	if len(raw) != len(results) {
		return fmt.Errorf("rpc: %s returned %d results, want %d", method, len(raw), len(results))
	}
	for i, r := range raw {
		if err := json.Unmarshal(r, results[i]); err != nil {
			return err
		}
	}
	return nil
}

// logErr records an error from a method that cannot return one.
func (s *Service) logErr(method string, err error) {
	if err != nil {
		s.log.Warn("rpc call", "method", method, "err", err)
	}
}

// SetMode switches the interface and resizes the window once the daemon
// has saved the mode.
func (s *Service) SetMode(mode string) error {
	if err := s.call(context.Background(), "SetMode", nil, mode); err != nil {
		return err
	}
	if s.onMode != nil {
		s.onMode(mode)
	}
	return nil
}

// GetSnapshot returns the daemon's state; when the daemon cannot be
// reached, a snapshot whose error says why, so the UI shows it.
func (s *Service) GetSnapshot() app.Snapshot {
	var sn app.Snapshot
	if err := s.call(context.Background(), "GetSnapshot", &sn); err != nil {
		code := rpc.CodeUnreachable
		for _, c := range []string{rpc.CodeNotAuthorized, rpc.CodeProtocol} {
			if strings.HasPrefix(err.Error(), c) {
				code = c
			}
		}
		s.log.Warn("daemon", "err", err)
		return unreachable(code)
	}
	return sn
}

// unreachable is the snapshot shown while the daemon cannot be used.
func unreachable(code string) app.Snapshot {
	return app.Snapshot{Status: app.StatusError, Error: &app.AppError{Code: code},
		Warnings: []app.AppError{}, Servers: []string{}, BlockedSites: []string{}}
}
