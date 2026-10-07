// Package rpc carries Ghostline's UI service between the Linux GUI and the
// root daemon: one JSON message per line over a Unix socket. It knows
// nothing about the app; it moves method names, JSON arguments, results,
// events and the reverse "ui" calls that open file dialogs in the GUI.
package rpc

import (
	"encoding/json"
)

// Protocol is bumped when a message changes incompatibly.
const Protocol = 1

// MaxLine bounds one message. Backups (8 MiB) travel base64-encoded in a
// reverse ui call, about 10.7 MiB.
const MaxLine = 16 << 20

// Error codes the UI translates (the same strings as in internal/app).
const (
	CodeUnreachable   = "DAEMON_UNREACHABLE"
	CodeProtocol      = "DAEMON_PROTOCOL_MISMATCH"
	CodeNotAuthorized = "NOT_AUTHORIZED"
	CodeNoUI          = "NO_UI"
)

// Hello is the first message each side sends.
type Hello struct {
	Version  string `json:"version"`
	Protocol int    `json:"protocol"`
}

// Error is a failed call. Detail is json.Marshal(&err) on the daemon,
// which is what Wails would have sent the frontend for that error.
type Error struct {
	Message string          `json:"message"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

// Msg is one line. Calls carry ID+Call+Args and are answered with
// ID+Result or ID+Error; events carry Event+Data; reverse ui calls carry
// UID+UI+Args and are answered with UID+Result or UID+Error.
type Msg struct {
	Hello  *Hello            `json:"hello,omitempty"`
	ID     uint64            `json:"id,omitempty"`
	Call   string            `json:"call,omitempty"`
	Args   []json.RawMessage `json:"args,omitempty"`
	Result json.RawMessage   `json:"result,omitempty"`
	Error  *Error            `json:"error,omitempty"`
	Event  string            `json:"event,omitempty"`
	Data   json.RawMessage   `json:"data,omitempty"`
	UID    uint64            `json:"uid,omitempty"`
	UI     string            `json:"ui,omitempty"`
}

// RemoteError is a daemon-side error as a call returns it: the same
// message, and the same JSON when Wails marshals it for the frontend.
type RemoteError struct {
	Message string
	Detail  json.RawMessage
}

func (e *RemoteError) Error() string { return e.Message }

// MarshalJSON returns the daemon's JSON for the original error.
func (e *RemoteError) MarshalJSON() ([]byte, error) {
	if len(e.Detail) == 0 {
		return []byte("{}"), nil
	}
	return e.Detail, nil
}

// errorOf encodes err the way Wails' default error marshaller does.
func errorOf(err error) *Error {
	e := &Error{Message: err.Error()}
	if b, jerr := json.Marshal(&err); jerr == nil {
		e.Detail = b
	}
	return e
}

func (e *Error) remote() error { return &RemoteError{Message: e.Message, Detail: e.Detail} }
