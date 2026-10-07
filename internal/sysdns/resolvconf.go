package sysdns

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/netwatch"
)

// resolvConfBackend rewrites /etc/resolv.conf: the last resort, for systems
// with neither NetworkManager nor systemd-resolved. Anything that later
// rewrites the file (dhcpcd, a new network) is taken as the new original
// and Ghostline's file is put back.
type resolvConfBackend struct {
	path   string
	backup string // a copy of the original, for RestoreDefault
	flush  func() error
	watch  netwatch.WatchFunc // must include watching path itself
}

func newResolvConf(path, dataDir string, flush func() error, watch netwatch.WatchFunc) Backend {
	return &resolvConfBackend{path: path, backup: filepath.Join(dataDir, "resolv.conf.orig"), flush: flush, watch: watch}
}

func (b *resolvConfBackend) Name() string { return "resolvconf" }

// current records the file as it is: a symlink, or contents and mode.
func (b *resolvConfBackend) current() (*model.ResolvConfFile, []string, error) {
	fi, err := os.Lstat(b.path)
	if errors.Is(err, fs.ErrNotExist) {
		return &model.ResolvConfFile{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	content, _ := os.ReadFile(b.path) // a dangling symlink has none
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(b.path)
		if err != nil {
			return nil, nil, err
		}
		return &model.ResolvConfFile{Symlink: target}, nameservers(content), nil
	}
	return &model.ResolvConfFile{Content: content, Mode: uint32(fi.Mode().Perm())}, nameservers(content), nil
}

func nameservers(content []byte) []string {
	out := []string{}
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && f[0] == "nameserver" {
			out = append(out, f[1])
		}
	}
	return notLoopback(out)
}

func (b *resolvConfBackend) Snapshot(Selection) (Snapshot, error) {
	rf, servers, err := b.current()
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Backend: b.Name(), Linux: &model.LinuxDNS{ResolvConf: rf, Servers: servers}}, nil
}

func resolvConfContent(v6 bool) []byte {
	c := marker + "\nnameserver 127.0.0.1\n"
	if v6 {
		c += "nameserver ::1\n"
	}
	return []byte(c + "options edns0 trust-ad\n")
}

func (b *resolvConfBackend) Apply(s Snapshot, v6 bool) error {
	if s.Linux != nil && s.Linux.ResolvConf != nil {
		saved, err := json.Marshal(s.Linux.ResolvConf)
		if err != nil {
			return err
		}
		if err := writeAtomic(b.backup, saved, 0o600); err != nil {
			return err
		}
	}
	return writeAtomic(b.path, resolvConfContent(v6), 0o644)
}

func (b *resolvConfBackend) ours() bool {
	got, err := os.ReadFile(b.path)
	return err == nil && bytes.HasPrefix(got, []byte(marker))
}

// Reconcile takes a file someone else wrote as the new original (a new
// network's DNS) and puts Ghostline's back.
func (b *resolvConfBackend) Reconcile(s Snapshot, _ Selection) (Snapshot, Snapshot, []Change, error) {
	if b.ours() {
		return s, Snapshot{}, nil, nil
	}
	next, err := b.Snapshot(Selection{})
	if err != nil {
		return s, Snapshot{}, nil, err
	}
	return next, next, []Change{{Target: b.path}}, nil
}

// StillOurs keeps s while the file is Ghostline's.
func (b *resolvConfBackend) StillOurs(s Snapshot) Snapshot {
	if b.ours() {
		return s
	}
	return Snapshot{}
}

// put writes rf back: the same symlink, or the same contents and mode;
// nothing at all if there was no file.
func (b *resolvConfBackend) put(rf *model.ResolvConfFile) error {
	switch {
	case rf.Symlink != "":
		tmp := b.path + ".ghostline.tmp"
		_ = os.Remove(tmp)
		if err := os.Symlink(rf.Symlink, tmp); err != nil {
			return err
		}
		return os.Rename(tmp, b.path)
	case rf.Content == nil && rf.Mode == 0:
		if err := os.Remove(b.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	default:
		return writeAtomic(b.path, rf.Content, os.FileMode(rf.Mode))
	}
}

func (b *resolvConfBackend) Restore(s Snapshot) []RestoreError {
	if s.Linux == nil || s.Linux.ResolvConf == nil {
		return nil
	}
	if err := b.put(s.Linux.ResolvConf); err != nil {
		return []RestoreError{{Target: b.path, Err: err}}
	}
	_ = os.Remove(b.backup)
	return nil
}

// RestoreDefault puts the saved original back when the file is
// Ghostline's; without a saved copy it reports an error rather than guess.
func (b *resolvConfBackend) RestoreDefault() error {
	if !b.ours() {
		return nil
	}
	saved, err := os.ReadFile(b.backup)
	if err != nil {
		return fmt.Errorf("resolvconf: no saved copy of %s: %w", b.path, err)
	}
	var rf model.ResolvConfFile
	if err := json.Unmarshal(saved, &rf); err != nil {
		return err
	}
	if err := b.put(&rf); err != nil {
		return err
	}
	_ = os.Remove(b.backup)
	return nil
}

func (b *resolvConfBackend) Flush() error { return b.flush() }

func (b *resolvConfBackend) Watch(onChange func()) (func(), error) {
	if b.watch == nil {
		return func() {}, nil
	}
	return b.watch(onChange)
}

func (b *resolvConfBackend) Info() Info {
	return Info{Backend: b.Name(), Chain: b.path, Interfaces: []string{}}
}
