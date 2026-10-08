// Package desktop sets the system proxy of a Linux desktop session (GNOME,
// KDE). Its tasks run in the user's session agent (ghostlined
// --session-agent); NewBackend is the daemon's side.
package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/session"
)

// runner runs a desktop tool and returns its output.
type runner func(name string, args ...string) ([]byte, error)

func execRun(name string, args ...string) ([]byte, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// desktop is one desktop's proxy settings, inside the user's session.
type desktop interface {
	snapshot(ours string) (model.ProxySnapshot, error)
	apply(addr string) error
	isOurs(addr string) (bool, error)
	restore(s model.ProxySnapshot) error
	watch(ctx context.Context, emit func()) error
}

// backendFor maps XDG_CURRENT_DESKTOP ("ubuntu:GNOME", "KDE", …) to the
// backend that owns its proxy settings; "" when none does.
func backendFor(xdgDesktop string) string {
	for _, part := range strings.Split(xdgDesktop, ":") {
		switch part {
		case "KDE":
			return "kde"
		case "GNOME", "X-Cinnamon", "Cinnamon", "Budgie", "Unity":
			return "gnome"
		}
	}
	return ""
}

// desktopNamed builds the real backend named backend for the user at home.
func desktopNamed(backend, home string) (desktop, bool) {
	switch backend {
	case "gnome":
		return gnome{run: execRun}, true
	case "kde":
		return newKDE(home), true
	}
	return nil, false
}

func unsupported(desk string) error {
	return &session.Error{Code: "PROXY_DESKTOP_UNSUPPORTED", Message: fmt.Sprintf("no proxy settings for desktop %q", desk)}
}

// AgentTasks are the proxy tasks the session agent serves.
func AgentTasks() map[string]session.Task { return agentTasks(os.Getenv, desktopNamed) }

// AgentStreams are the proxy watches the session agent serves.
func AgentStreams() map[string]session.StreamTask {
	return map[string]session.StreamTask{
		"proxy.watch": func(ctx context.Context, _ json.RawMessage, emit func(any)) error {
			d, ok := desktopNamed(backendFor(os.Getenv("XDG_CURRENT_DESKTOP")), os.Getenv("HOME"))
			if !ok {
				return unsupported(os.Getenv("XDG_CURRENT_DESKTOP"))
			}
			return d.watch(ctx, func() { emit("changed") })
		},
	}
}

type addrArgs struct {
	Ours string `json:"ours,omitempty"`
	Addr string `json:"addr,omitempty"`
}

type restoreArgs struct {
	Snapshot model.ProxySnapshot `json:"snapshot"`
}

func agentTasks(env func(string) string, named func(backend, home string) (desktop, bool)) map[string]session.Task {
	current := func() (desktop, error) {
		d, ok := named(backendFor(env("XDG_CURRENT_DESKTOP")), env("HOME"))
		if !ok {
			return nil, unsupported(env("XDG_CURRENT_DESKTOP"))
		}
		return d, nil
	}
	withAddr := func(f func(d desktop, a addrArgs) (any, error)) session.Task {
		return func(raw json.RawMessage) (any, error) {
			var a addrArgs
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &a); err != nil {
					return nil, err
				}
			}
			d, err := current()
			if err != nil {
				return nil, err
			}
			return f(d, a)
		}
	}
	return map[string]session.Task{
		"proxy.snapshot": withAddr(func(d desktop, a addrArgs) (any, error) { return d.snapshot(a.Ours) }),
		"proxy.apply":    withAddr(func(d desktop, a addrArgs) (any, error) { return nil, d.apply(a.Addr) }),
		"proxy.isOurs":   withAddr(func(d desktop, a addrArgs) (any, error) { return d.isOurs(a.Addr) }),
		"proxy.restore": func(raw json.RawMessage) (any, error) {
			var a restoreArgs
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, err
			}
			d, ok := named(a.Snapshot.Backend, env("HOME"))
			if !ok {
				return nil, unsupported(a.Snapshot.Backend)
			}
			return nil, d.restore(a.Snapshot)
		},
	}
}
