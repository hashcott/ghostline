// Package session runs fixed tasks inside a user's desktop session for the
// root daemon: the daemon starts its own binary with --session-agent as the
// user, sends one JSON request on stdin and reads the reply on stdout. Work
// for a user who is not logged in waits in a Queue.
package session

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// User is a desktop session's owner, as the agent runs as them.
type User struct {
	UID, GID int
	Groups   []int
	Name     string
	Home     string
	Desktop  string // XDG_CURRENT_DESKTOP of their session ("KDE", "GNOME", …)
}

// Task runs once in the agent and returns its result.
type Task func(args json.RawMessage) (any, error)

// StreamTask runs in the agent until ctx ends (the daemon closed stdin),
// emitting one line per event.
type StreamTask func(ctx context.Context, args json.RawMessage, emit func(any)) error

// Sessions finds users' desktop sessions and runs agent tasks in them.
type Sessions interface {
	// Active is the owner of the graphical session in front of seat0.
	Active() (User, bool)
	// ByUID is uid with a live session (its session bus), for restores.
	ByUID(uid int) (User, bool)
	Run(u User, task string, in, out any) error
	Stream(u User, task string, in any, onLine func(json.RawMessage)) (stop func(), err error)
	// WatchNew calls onNew when a user logs in.
	WatchNew(onNew func(User)) (stop func(), err error)
}

// ErrAgent is any failure reported by an agent task.
var ErrAgent = errors.New("session: agent task failed")

// Error is an agent task's failure, with a code the daemon can map.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string        { return fmt.Sprintf("session: %s: %s", e.Code, e.Message) }
func (e *Error) Is(target error) bool { return target == ErrAgent }

type request struct {
	Task string          `json:"task"`
	Args json.RawMessage `json:"args,omitempty"`
}

type reply struct {
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Serve is the agent side: it reads one request, runs the task (a stream
// until in ends) and writes the reply. Exit code: 0 ok, 1 the task failed,
// 2 the request was refused.
func Serve(in io.Reader, out io.Writer, tasks map[string]Task, streams map[string]StreamTask) int {
	br := bufio.NewReader(in)
	line, err := br.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return 2
	}
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		writeReply(out, reply{Error: &Error{Code: "BAD_REQUEST", Message: err.Error()}})
		return 2
	}
	if st, ok := streams[req.Task]; ok {
		ctx, cancel := context.WithCancel(context.Background())
		go func() { _, _ = io.Copy(io.Discard, br); cancel() }() // stdin closed: stop
		enc := json.NewEncoder(out)
		if err := st(ctx, req.Args, func(v any) { _ = enc.Encode(reply{Result: v}) }); err != nil {
			writeReply(out, reply{Error: asError(err)})
			return 1
		}
		return 0
	}
	t, ok := tasks[req.Task]
	if !ok {
		writeReply(out, reply{Error: &Error{Code: "UNKNOWN_TASK", Message: req.Task}})
		return 2
	}
	res, err := t(req.Args)
	if err != nil {
		writeReply(out, reply{Error: asError(err)})
		return 1
	}
	writeReply(out, reply{Result: res})
	return 0
}

func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return &Error{Code: "TASK_FAILED", Message: err.Error()}
}

func writeReply(w io.Writer, r reply) { _ = json.NewEncoder(w).Encode(r) }
