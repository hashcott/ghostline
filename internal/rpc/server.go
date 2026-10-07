package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
)

// Handler runs one call: method with its JSON arguments, returning the
// JSON result.
type Handler func(ctx context.Context, method string, args []json.RawMessage) (json.RawMessage, error)

// Server serves calls and pushes events to every connected client.
type Server struct {
	h         Handler
	version   string
	authorize func(net.Conn) error
	log       *slog.Logger

	mu     sync.Mutex
	conns  map[*sconn]struct{}
	lns    []net.Listener
	closed bool
}

// NewServer serves h. authorize decides each connection before anything is
// read from it.
func NewServer(h Handler, version string, authorize func(net.Conn) error, log *slog.Logger) *Server {
	return &Server{h: h, version: version, authorize: authorize, log: log, conns: map[*sconn]struct{}{}}
}

// Serve accepts connections on l until Close.
func (s *Server) Serve(l net.Listener) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return net.ErrClosed
	}
	s.lns = append(s.lns, l)
	s.mu.Unlock()
	for {
		c, err := l.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			return err
		}
		go s.handle(c)
	}
}

// Emit sends an event to every connected client (app.Emitter).
func (s *Server) Emit(name string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		s.log.Warn("rpc: event", "name", name, "err", err)
		return
	}
	m := Msg{Event: name, Data: b}
	s.mu.Lock()
	conns := make([]*sconn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.w.write(m)
	}
}

// Close stops the listeners and drops every connection.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	lns, conns := s.lns, s.conns
	s.lns, s.conns = nil, map[*sconn]struct{}{}
	s.mu.Unlock()
	for _, l := range lns {
		_ = l.Close()
	}
	for c := range conns {
		_ = c.w.c.Close()
	}
	return nil
}

// sconn is one client connection on the server side.
type sconn struct {
	w *wire

	mu      sync.Mutex
	nextUID uint64
	waiting map[uint64]chan Msg // reverse ui calls awaiting an answer
	gone    chan struct{}
}

func (s *Server) handle(c net.Conn) {
	w := newWire(c)
	defer c.Close()
	if err := s.authorize(c); err != nil {
		msg := err.Error()
		if !strings.HasPrefix(msg, CodeNotAuthorized) {
			msg = CodeNotAuthorized + ": " + msg
		}
		_ = w.write(Msg{Error: &Error{Message: msg}})
		return
	}
	if err := w.write(Msg{Hello: &Hello{Version: s.version, Protocol: Protocol}}); err != nil {
		return
	}
	first, err := w.read()
	if err != nil || first.Hello == nil {
		return
	}
	if first.Hello.Protocol != Protocol {
		_ = w.write(Msg{Error: &Error{Message: fmt.Sprintf("%s: daemon %d, client %d", CodeProtocol, Protocol, first.Hello.Protocol)}})
		return
	}
	sc := &sconn{w: w, waiting: map[uint64]chan Msg{}, gone: make(chan struct{})}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.conns[sc] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, sc)
		s.mu.Unlock()
		close(sc.gone)
	}()
	for {
		m, err := w.read()
		if err != nil {
			if errors.Is(err, ErrLineTooLong) {
				s.log.Warn("rpc: message too long; closing")
			}
			return
		}
		switch {
		case m.Call != "":
			go s.call(sc, m)
		case m.UID != 0:
			sc.mu.Lock()
			ch := sc.waiting[m.UID]
			delete(sc.waiting, m.UID)
			sc.mu.Unlock()
			if ch != nil {
				ch <- m
			}
		}
	}
}

type connKey struct{}

func (s *Server) call(sc *sconn, m Msg) {
	ctx := context.WithValue(context.Background(), connKey{}, sc)
	res, err := s.h(ctx, m.Call, m.Args)
	out := Msg{ID: m.ID}
	if err != nil {
		out.Error = errorOf(err)
	} else {
		if res == nil {
			res = json.RawMessage("null")
		}
		out.Result = res
	}
	if werr := sc.w.write(out); werr != nil {
		s.log.Warn("rpc: reply", "call", m.Call, "err", werr)
	}
}
