package desktop

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/session"
	"github.com/hashcott/ghostline/internal/sysproxy"
)

// backend is the daemon's side: it runs the proxy tasks in the session of
// the user at the screen, and restores for the user the proxy was set for.
type backend struct {
	s session.Sessions
	q *session.Queue
}

// NewBackend sets the system proxy of the graphical session's desktop;
// restores for a user who is not logged in wait in q.
func NewBackend(s session.Sessions, q *session.Queue) sysproxy.Backend { return backend{s: s, q: q} }

func (b backend) active() (session.User, error) {
	u, ok := b.s.Active()
	if !ok {
		return session.User{}, sysproxy.ErrNoSession
	}
	if backendFor(u.Desktop) == "" {
		return u, fmt.Errorf("%w: %q", sysproxy.ErrDesktopUnsupported, u.Desktop)
	}
	return u, nil
}

func (b backend) Snapshot(ours string) (sysproxy.Snapshot, error) {
	u, err := b.active()
	if err != nil {
		return sysproxy.Snapshot{}, err
	}
	var s sysproxy.Snapshot
	if err := b.s.Run(u, "proxy.snapshot", addrArgs{Ours: ours}, &s); err != nil {
		return sysproxy.Snapshot{}, err
	}
	s.UID = u.UID
	return s, nil
}

func unquote(v string) string { return strings.Trim(v, "'") }

// Existing reports a proxy server or PAC script the user had enabled.
func (backend) Existing(s sysproxy.Snapshot) (server, pac string, has bool) {
	switch {
	case s.KDE != nil:
		v := s.KDE.Values
		switch v["ProxyType"] {
		case "1":
			server = strings.Replace(strings.TrimPrefix(strings.TrimPrefix(v["httpProxy"], "http://"), "https://"), " ", ":", 1)
		case "2":
			pac = v["Proxy Config Script"]
		}
	case s.GNOME != nil:
		v := s.GNOME.Values
		switch unquote(v["org.gnome.system.proxy mode"]) {
		case "manual":
			if h := unquote(v["org.gnome.system.proxy.http host"]); h != "" {
				server = h + ":" + v["org.gnome.system.proxy.http port"]
			}
		case "auto":
			pac = unquote(v["org.gnome.system.proxy autoconfig-url"])
		}
	}
	return server, pac, server != "" || pac != ""
}

func (b backend) Apply(addr string) error {
	u, err := b.active()
	if err != nil {
		return err
	}
	if err := b.s.Run(u, "proxy.apply", addrArgs{Addr: addr}, nil); err != nil {
		return err
	}
	var ours bool
	if err := b.s.Run(u, "proxy.isOurs", addrArgs{Addr: addr}, &ours); err != nil {
		return err
	}
	if !ours {
		return sysproxy.ErrNotApplied
	}
	return nil
}

func (b backend) IsOurs(addr string) (bool, error) {
	u, err := b.active()
	if err != nil {
		return false, err
	}
	var ours bool
	return ours, b.s.Run(u, "proxy.isOurs", addrArgs{Addr: addr}, &ours)
}

// queuedRestore is a restore waiting for its user to log in.
type queuedRestore struct {
	Addr     string              `json:"addr"`
	Snapshot model.ProxySnapshot `json:"snapshot"`
}

// RestoreIfOurs restores for the user the snapshot was taken for. Not
// logged in: the restore is queued (and counts as done here).
func (b backend) RestoreIfOurs(addr string, s sysproxy.Snapshot) (bool, error) {
	u, ok := b.s.ByUID(s.UID)
	if !ok {
		if err := b.q.Add(s.UID, "proxy.restore", queuedRestore{Addr: addr, Snapshot: s}); err != nil {
			return false, err
		}
		return true, nil
	}
	return restoreFor(b.s, u, addr, s)
}

func restoreFor(s session.Sessions, u session.User, addr string, snap sysproxy.Snapshot) (bool, error) {
	var ours bool
	if err := s.Run(u, "proxy.isOurs", addrArgs{Addr: addr}, &ours); err == nil && !ours {
		return false, nil // the user or another app changed it since
	}
	if err := s.Run(u, "proxy.restore", restoreArgs{Snapshot: snap}, nil); err != nil {
		return false, err
	}
	return true, nil
}

// QueueHandler runs queued proxy restores; other tasks stay queued.
func QueueHandler(s session.Sessions) func(task string, args json.RawMessage) error {
	return func(task string, args json.RawMessage) error {
		if task != "proxy.restore" {
			return fmt.Errorf("desktop: not a proxy task: %s", task)
		}
		var r queuedRestore
		if err := json.Unmarshal(args, &r); err != nil {
			return err
		}
		u, ok := s.ByUID(r.Snapshot.UID)
		if !ok {
			return sysproxy.ErrNoSession
		}
		_, err := restoreFor(s, u, r.Addr, r.Snapshot)
		return err
	}
}

// Watch follows the graphical session's proxy settings, and the next
// session's after a login.
func (b backend) Watch(onChange func()) (func(), error) {
	var mu sync.Mutex
	stopStream := func() {}
	start := func() {
		mu.Lock()
		defer mu.Unlock()
		stopStream()
		stopStream = func() {}
		if u, err := b.active(); err == nil {
			if stop, err := b.s.Stream(u, "proxy.watch", nil, func(json.RawMessage) { onChange() }); err == nil {
				stopStream = stop
			}
		}
	}
	start()
	stopNew, err := b.s.WatchNew(func(session.User) { start() })
	if err != nil {
		stopNew = func() {}
	}
	return func() {
		stopNew()
		mu.Lock()
		defer mu.Unlock()
		stopStream()
	}, nil
}

func (b backend) Info() sysproxy.Info {
	u, ok := b.s.Active()
	if !ok {
		return sysproxy.Info{}
	}
	switch backendFor(u.Desktop) {
	case "kde":
		return sysproxy.Info{Desktop: "KDE", Supported: true}
	case "gnome":
		return sysproxy.Info{Desktop: "GNOME", Supported: true}
	}
	return sysproxy.Info{Desktop: u.Desktop}
}
