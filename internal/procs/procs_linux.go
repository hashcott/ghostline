package procs

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// clockTicks is USER_HZ, 100 on every Linux architecture Ghostline ships.
const clockTicks = 100

type linuxInspector struct{}

// NewLinux inspects processes through /proc and stops services through
// systemd.
func NewLinux() Inspector { return linuxInspector{} }

func (linuxInspector) IsAdmin() bool { return os.Geteuid() == 0 }

// PortOwners lists the processes with a socket on local port: listening
// TCP, or any UDP, over IPv4 and IPv6.
func (linuxInspector) PortOwners(port uint16) ([]PortOwner, error) {
	proto := map[uint64]string{}
	for _, t := range []struct {
		file, proto string
		tcp         bool
	}{{"/proc/net/tcp", "tcp", true}, {"/proc/net/tcp6", "tcp", true}, {"/proc/net/udp", "udp", false}, {"/proc/net/udp6", "udp", false}} {
		data, err := os.ReadFile(t.file)
		if err != nil {
			continue // no IPv6, for instance
		}
		for _, ino := range parseProcNet(data, port, t.tcp) {
			proto[ino] = t.proto
		}
	}
	if len(proto) == 0 {
		return nil, nil
	}
	pids, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []PortOwner
	for _, d := range pids {
		pid, err := strconv.ParseUint(d.Name(), 10, 32)
		if err != nil {
			continue
		}
		fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
		if err != nil {
			continue // another user's process, or gone
		}
		for _, fd := range fds {
			link, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			ino, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]"), 10, 64)
			if err != nil {
				continue
			}
			p, ok := proto[ino]
			key := fmt.Sprintf("%d/%s", pid, p)
			if !ok || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, PortOwner{PID: uint32(pid), Name: procName(uint32(pid)), Service: procUnit(uint32(pid)), Proto: p})
		}
	}
	return out, nil
}

// ProcessNames lists the command names of the processes in /proc.
func (linuxInspector) ProcessNames() ([]string, error) {
	ds, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range ds {
		pid, err := strconv.ParseUint(d.Name(), 10, 32)
		if err != nil {
			continue
		}
		if n := procName(uint32(pid)); n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

func procName(pid uint32) string {
	b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	return strings.TrimSpace(string(b))
}

// procUnit is the system service a process runs in (from its cgroup).
func procUnit(pid uint32) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return ""
	}
	return unitFromCgroup(string(b))
}

// StopService stops a systemd unit and waits for it; never DNS itself or
// Ghostline (stoppable).
func (linuxInspector) StopService(name string, wait time.Duration) error {
	if err := stoppable(name); err != nil {
		return err
	}
	return stopUnit(name, wait)
}

// StartTime is when pid started: boot time plus field 22 of /proc/<pid>/stat.
func (linuxInspector) StartTime(pid uint32) (time.Time, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return time.Time{}, err
	}
	// The command name (field 2) is in parentheses and may contain spaces.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return time.Time{}, errors.New("procs: malformed stat")
	}
	fields := strings.Fields(s[i+1:]) // fields 3, 4, …
	if len(fields) < 20 {
		return time.Time{}, errors.New("procs: short stat")
	}
	ticks, err := strconv.ParseUint(fields[22-3], 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	boot, err := bootTime()
	if err != nil {
		return time.Time{}, err
	}
	return boot.Add(time.Duration(ticks) * time.Second / clockTicks), nil
}

// Alive reports whether pid runs and started at start (within a second:
// boot time is whole seconds), so a reused PID does not count.
func (l linuxInspector) Alive(pid uint32, start time.Time) bool {
	st, err := l.StartTime(pid)
	if err != nil {
		return false
	}
	d := st.Sub(start)
	return d > -time.Second && d < time.Second
}

// WaitForExit blocks until pid exits (a pidfd becomes readable).
func (linuxInspector) WaitForExit(pid uint32) error {
	fd, err := unix.PidfdOpen(int(pid), 0)
	if errors.Is(err, unix.ESRCH) {
		return nil // already gone
	}
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	for {
		_, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, -1)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

func bootTime() (time.Time, error) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "btime "); ok {
			secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, err
			}
			return time.Unix(secs, 0), nil
		}
	}
	return time.Time{}, errors.New("procs: no btime in /proc/stat")
}
