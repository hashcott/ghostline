package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
)

// ErrLineTooLong means a peer sent a message longer than MaxLine.
var ErrLineTooLong = errors.New("rpc: message too long")

// wire reads and writes one message per line on a connection. Writes may
// come from many goroutines.
type wire struct {
	c   net.Conn
	sc  *bufio.Scanner
	wmu sync.Mutex
}

func newWire(c net.Conn) *wire {
	sc := bufio.NewScanner(c)
	// The buffer grows as needed up to one full message plus its newline;
	// a longer line stops the scanner at once.
	sc.Buffer(make([]byte, 64<<10), MaxLine+1)
	return &wire{c: c, sc: sc}
}

// read returns the next message; a line over MaxLine is ErrLineTooLong.
func (w *wire) read() (Msg, error) {
	if !w.sc.Scan() {
		err := w.sc.Err()
		if errors.Is(err, bufio.ErrTooLong) {
			return Msg{}, ErrLineTooLong
		}
		if err == nil {
			err = io.EOF
		}
		return Msg{}, err
	}
	var m Msg
	if err := json.Unmarshal(w.sc.Bytes(), &m); err != nil {
		return Msg{}, err
	}
	return m, nil
}

func (w *wire) write(m Msg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(b) > MaxLine {
		return ErrLineTooLong
	}
	b = append(b, '\n')
	w.wmu.Lock()
	defer w.wmu.Unlock()
	_, err = w.c.Write(b)
	return err
}

// encodeArgs marshals each argument on its own.
func encodeArgs(args []any) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, len(args))
	for i, a := range args {
		b, err := json.Marshal(a)
		if err != nil {
			return nil, err
		}
		out[i] = b
	}
	return out, nil
}
