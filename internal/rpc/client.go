package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
)

// Client is one connection to the daemon.
type Client struct {
	w *wire
	// daemonVersion is the build the daemon named in its hello.
	daemonVersion string

	mu      sync.Mutex
	nextID  uint64
	pending map[uint64]chan Msg
	onEvent func(name string, data json.RawMessage)
	onUI    func(ctx context.Context, kind string, args []json.RawMessage) (any, error)
	done    chan struct{}
	err     error
	// events are handed to onEvent in order on their own goroutine: a
	// handler may call the daemon, whose reply the read loop must deliver.
	events chan Msg
}

// eventQueue bounds events waiting for a slow handler; past it they are
// dropped (the next state event brings the UI up to date).
const eventQueue = 4096

// Dial connects to the daemon and exchanges hellos. A refused peer gives
// an error starting with CodeNotAuthorized, a different protocol one
// starting with CodeProtocol.
func Dial(ctx context.Context, socket, version string) (*Client, error) {
	var d net.Dialer
	c, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", CodeUnreachable, err)
	}
	w := newWire(c)
	first, err := w.read()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("%s: %w", CodeUnreachable, err)
	}
	if first.Error != nil {
		c.Close()
		return nil, errors.New(first.Error.Message)
	}
	if first.Hello == nil || first.Hello.Protocol != Protocol {
		c.Close()
		got := 0
		if first.Hello != nil {
			got = first.Hello.Protocol
		}
		return nil, fmt.Errorf("%s: daemon %d, client %d", CodeProtocol, got, Protocol)
	}
	if err := w.write(Msg{Hello: &Hello{Version: version, Protocol: Protocol}}); err != nil {
		c.Close()
		return nil, fmt.Errorf("%s: %w", CodeUnreachable, err)
	}
	cl := &Client{w: w, daemonVersion: first.Hello.Version, pending: map[uint64]chan Msg{}, done: make(chan struct{}), events: make(chan Msg, eventQueue)}
	go cl.loop()
	go cl.dispatch()
	return cl, nil
}

// OnEvent sets the event handler.
func (c *Client) OnEvent(fn func(name string, data json.RawMessage)) {
	c.mu.Lock()
	c.onEvent = fn
	c.mu.Unlock()
}

// OnUI sets the handler for the daemon's reverse ui calls (file dialogs).
func (c *Client) OnUI(fn func(ctx context.Context, kind string, args []json.RawMessage) (any, error)) {
	c.mu.Lock()
	c.onUI = fn
	c.mu.Unlock()
}

// DaemonVersion is the daemon's build, from its hello.
func (c *Client) DaemonVersion() string { return c.daemonVersion }

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close ends the connection.
func (c *Client) Close() error { return c.w.c.Close() }

// Call runs method on the daemon and decodes its result into result (nil
// to drop it). A daemon-side error is a *RemoteError.
func (c *Client) Call(ctx context.Context, method string, result any, args ...any) error {
	raw, err := encodeArgs(args)
	if err != nil {
		return err
	}
	ch := make(chan Msg, 1)
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := c.nextID
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.w.write(Msg{ID: id, Call: method, Args: raw}); err != nil {
		c.forget(id)
		return fmt.Errorf("%s: %w", CodeUnreachable, err)
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return m.Error.remote()
		}
		if result != nil && len(m.Result) > 0 && string(m.Result) != "null" {
			return json.Unmarshal(m.Result, result)
		}
		return nil
	case <-c.done:
		c.mu.Lock()
		err := c.err
		c.mu.Unlock()
		return err
	case <-ctx.Done():
		c.forget(id)
		return ctx.Err()
	}
}

func (c *Client) forget(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) loop() {
	var err error
	defer func() {
		c.mu.Lock()
		c.err = fmt.Errorf("%s: connection closed: %v", CodeUnreachable, err)
		c.mu.Unlock()
		close(c.done)
		_ = c.w.c.Close()
	}()
	for {
		var m Msg
		m, err = c.w.read()
		if err != nil {
			return
		}
		switch {
		case m.Event != "":
			select {
			case c.events <- m:
			default: // the handler is stuck; never stall the replies behind it
			}
		case m.UI != "":
			go c.answerUI(m)
		case m.ID != 0:
			c.mu.Lock()
			ch := c.pending[m.ID]
			delete(c.pending, m.ID)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
	}
}

// dispatch runs the event handler for queued events, in order, until the
// connection ends.
func (c *Client) dispatch() {
	for {
		select {
		case m := <-c.events:
			c.mu.Lock()
			fn := c.onEvent
			c.mu.Unlock()
			if fn != nil {
				fn(m.Event, m.Data)
			}
		case <-c.done:
			return
		}
	}
}

func (c *Client) answerUI(m Msg) {
	c.mu.Lock()
	fn := c.onUI
	c.mu.Unlock()
	out := Msg{UID: m.UID}
	if fn == nil {
		out.Error = &Error{Message: CodeNoUI}
	} else if res, err := fn(context.Background(), m.UI, m.Args); err != nil {
		out.Error = errorOf(err)
	} else if b, err := json.Marshal(res); err != nil {
		out.Error = errorOf(err)
	} else {
		out.Result = b
	}
	_ = c.w.write(out)
}
