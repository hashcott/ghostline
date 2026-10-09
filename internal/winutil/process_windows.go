package winutil

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// IsAdmin reports whether the current process token is elevated.
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func openQuery(pid uint32) (windows.Handle, error) {
	return windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
}

// ProcessStartTime returns the creation time of pid.
func ProcessStartTime(pid uint32) (time.Time, error) {
	h, err := openQuery(pid)
	if err != nil {
		return time.Time{}, err
	}
	defer windows.CloseHandle(h)
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, c.Nanoseconds()), nil
}

// ProcessAlive reports whether pid is running and was created at start
// (±1ms), so a reused PID is not mistaken for the original process.
func ProcessAlive(pid uint32, start time.Time) bool {
	h, err := openQuery(pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil || code != 259 /* STILL_ACTIVE */ {
		return false
	}
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return false
	}
	d := time.Unix(0, c.Nanoseconds()).Sub(start)
	return d > -time.Millisecond && d < time.Millisecond
}

// ProcessName returns the full image path of pid.
func ProcessName(pid uint32) (string, error) {
	h, err := openQuery(pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// ProcessNames lists the image names (such as "svchost.exe") of the
// running processes.
func ProcessNames() ([]string, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var names []string
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		names = append(names, windows.UTF16ToString(e.ExeFile[:]))
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return names, err
	}
	return names, nil
}

// WaitForExit blocks until pid exits (or cannot be opened).
func WaitForExit(pid uint32) error {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		// ERROR_INVALID_PARAMETER: no such pid. Anything else (access
		// denied) means we are not really waiting.
		if !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			slog.Warn("winutil: open process to wait for exit failed; treating it as exited", "err", err, "pid", pid)
		}
		return nil // already gone
	}
	defer windows.CloseHandle(h)
	_, err = windows.WaitForSingleObject(h, windows.INFINITE)
	return err
}

// HiddenCmd builds a command that runs without a console window.
func HiddenCmd(exe string, args []string, dir string) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}

// Job is a Windows job object that kills its processes when closed.
type Job struct{ h windows.Handle }

// NewKillOnCloseJob creates a job with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE.
func NewKillOnCloseJob() (*Job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &Job{h: h}, nil
}

// Assign puts p into the job.
func (j *Job) Assign(p *os.Process) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(j.h, h)
}

// Close closes the job handle, killing its processes.
func (j *Job) Close() error { return windows.CloseHandle(j.h) }
