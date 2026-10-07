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

// NewLinux inspects processes through /proc. Port owners and stopping a
// service come with the Linux DNS backends (L3).
func NewLinux() Inspector { return linuxInspector{} }

func (linuxInspector) IsAdmin() bool { return os.Geteuid() == 0 }

func (linuxInspector) PortOwners(uint16) ([]PortOwner, error) { return nil, errUnsupported }

func (linuxInspector) StopService(string, time.Duration) error { return errUnsupported }

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
