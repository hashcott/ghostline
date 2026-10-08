package dpi

import (
	"os/exec"
	"sync/atomic"
	"syscall"
)

type linuxProc struct {
	cmd    *exec.Cmd
	exited atomic.Bool
}

func (p *linuxProc) PID() int     { return p.cmd.Process.Pid }
func (p *linuxProc) Exited() bool { return p.exited.Load() }

// Kill ends the engine's whole process group: anything it forked goes too.
func (p *linuxProc) Kill() error { return syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL) }

type linuxRunner struct{}

// NewLinuxRunner starts the engine as a direct child in its own process
// group. Pdeathsig does not survive nfqws2 dropping to nobody, so the
// daemon's death is covered by systemd's KillMode=control-group, and Stop
// kills the group. Its output is discarded (nfqws2 is quiet without
// --debug).
func NewLinuxRunner() Runner { return linuxRunner{} }

func (linuxRunner) Start(exe string, args []string, dir string) (Process, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL, Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &linuxProc{cmd: cmd}
	go func() { _ = cmd.Wait(); p.exited.Store(true) }()
	return p, nil
}
