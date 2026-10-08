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
func (p *linuxProc) Kill() error  { return p.cmd.Process.Kill() }

type linuxRunner struct{}

// NewLinuxRunner starts the engine as a direct child that the kernel kills
// when the daemon dies (Pdeathsig); systemd's KillMode=control-group is the
// second line. Its output is discarded (nfqws2 is quiet without --debug).
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
