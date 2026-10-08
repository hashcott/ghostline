package desktop

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"golang.org/x/sys/unix"
)

// kdeKeys are the [Proxy Settings] keys of kioslaverc Ghostline records.
var kdeKeys = []string{"ProxyType", "httpProxy", "httpsProxy", "ftpProxy", "socksProxy", "NoProxyFor", "Proxy Config Script", "ReversedException"}

const kioGroup = "Proxy Settings"

// kde sets KDE Plasma's proxy (kioslaverc) through kwriteconfig, then tells
// KIO to re-read it.
type kde struct {
	run     runner
	read    func() ([]byte, error)
	tool    string // kwriteconfig6 or kwriteconfig5
	reparse func() error
	dir     string // ~/.config, watched
}

func newKDE(home string) kde {
	dir := filepath.Join(home, ".config")
	tool := "kwriteconfig6"
	if _, err := exec.LookPath(tool); err != nil {
		tool = "kwriteconfig5"
	}
	return kde{run: execRun, read: fileReader(filepath.Join(dir, "kioslaverc")), tool: tool, reparse: reparseKIO, dir: dir}
}

func fileReader(path string) func() ([]byte, error) {
	return func() ([]byte, error) { return os.ReadFile(path) }
}

// readKIO returns the [Proxy Settings] keys of kioslaverc and which exist;
// no file is no keys.
func readKIO(read func() ([]byte, error)) (map[string]string, []string, error) {
	b, err := read()
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	vals, present := map[string]string{}, []string{}
	in := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			in = line == "["+kioGroup+"]"
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !in || !ok {
			continue
		}
		k = strings.TrimSuffix(strings.TrimSpace(k), "[$e]")
		if slices.Contains(kdeKeys, k) {
			vals[k] = kconfigUnescape(v)
			present = append(present, k)
		}
	}
	return vals, present, nil
}

// kconfigUnescape decodes a value as KConfig reads it (printableToString):
// \s \t \n \r \\ and \xNN; \; and \, stay as they are (list
// separators), as does any other backslash. kwriteconfig takes the decoded
// value and escapes it again.
func kconfigUnescape(v string) string {
	if !strings.Contains(v, `\`) {
		return v
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 == len(v) {
			b.WriteByte(v[i])
			continue
		}
		i++
		switch c := v[i]; c {
		case 's':
			b.WriteByte(' ')
		case 't':
			b.WriteByte('\t')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		case 'x':
			if i+2 < len(v) {
				if n, err := strconv.ParseUint(v[i+1:i+3], 16, 8); err == nil {
					b.WriteByte(byte(n))
					i += 2
					continue
				}
			}
			b.WriteString(`\x`)
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
		}
	}
	return b.String()
}

// kdeAddr is how kioslaverc spells a proxy: "http://host port".
func kdeAddr(addr string) string {
	host, port, _ := net.SplitHostPort(addr)
	return "http://" + host + " " + port
}

func (k kde) write(key, v string) error {
	_, err := k.run(k.tool, "--file", "kioslaverc", "--group", kioGroup, "--key", key, v)
	return err
}

func (k kde) del(key string) error {
	_, err := k.run(k.tool, "--file", "kioslaverc", "--group", kioGroup, "--key", key, "--delete")
	return err
}

func (k kde) isOurs(addr string) (bool, error) {
	vals, _, err := readKIO(k.read)
	if err != nil {
		return false, err
	}
	return vals["ProxyType"] == "1" && vals["httpProxy"] == kdeAddr(addr), nil
}

// snapshot records the keys; Ghostline's own leftover means "none set".
func (k kde) snapshot(ours string) (model.ProxySnapshot, error) {
	vals, present, err := readKIO(k.read)
	if err != nil {
		return model.ProxySnapshot{}, err
	}
	if vals["ProxyType"] == "1" && vals["httpProxy"] == kdeAddr(ours) {
		vals, present = map[string]string{}, nil
	}
	return model.ProxySnapshot{Backend: "kde", KDE: &model.KDEProxy{Values: vals, Present: present}}, nil
}

func (k kde) apply(addr string) error {
	for _, kv := range [][2]string{
		{"httpProxy", kdeAddr(addr)},
		{"httpsProxy", kdeAddr(addr)},
		{"NoProxyFor", sysproxy.BypassKDE()},
		{"ReversedException", "false"},
		{"ProxyType", "1"}, // last: the addresses first
	} {
		if err := k.write(kv[0], kv[1]); err != nil {
			return err
		}
	}
	return k.reparse()
}

// restore writes back the recorded keys and deletes the ones that did not
// exist.
func (k kde) restore(s model.ProxySnapshot) error {
	p := &model.KDEProxy{}
	if s.KDE != nil {
		p = s.KDE
	}
	for _, key := range kdeKeys {
		var err error
		if slices.Contains(p.Present, key) {
			err = k.write(key, p.Values[key])
		} else {
			err = k.del(key)
		}
		if err != nil {
			return err
		}
	}
	return k.reparse()
}

func (k kde) watch(ctx context.Context, emit func()) error {
	return watchKIO(ctx, k.dir, time.Second, emit)
}

// reparseKIO tells running KDE apps to re-read the proxy settings.
func reparseKIO() error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Emit("/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "")
}

// watchKIO emits (debounced by delay) when kioslaverc in dir is written or
// replaced, until ctx ends.
func watchKIO(ctx context.Context, dir string, delay time.Duration, emit func()) error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if _, err := unix.InotifyAddWatch(fd, dir, unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO|unix.IN_CREATE|unix.IN_DELETE); err != nil {
		return err
	}
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	buf := make([]byte, 4096)
	for ctx.Err() == nil {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		if n, err := unix.Poll(fds, 250); err != nil && !errors.Is(err, unix.EINTR) {
			return err
		} else if n == 0 {
			continue
		}
		n, err := unix.Read(fd, buf)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EINTR) {
				continue
			}
			return err
		}
		for off := 0; off+unix.SizeofInotifyEvent <= n; {
			ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
			name := strings.TrimRight(string(buf[off+unix.SizeofInotifyEvent:off+unix.SizeofInotifyEvent+int(ev.Len)]), "\x00")
			off += unix.SizeofInotifyEvent + int(ev.Len)
			if name == "kioslaverc" {
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(delay, emit)
			}
		}
	}
	return nil
}
