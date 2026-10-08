package startup

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
)

// Create registers (or replaces) the task. Needs elevation for HighestAvailable.
func Create(t Task) error { return createXML(t.Name, TaskXML(t)) }

// CreateGuard writes guard.ps1 into dir, made admin-only first because the
// task runs it as SYSTEM, and registers the guard task for the current
// user's state.json. Needs elevation.
func CreateGuard(dir, state string) error {
	if err := winutil.SecureDir(dir); err != nil {
		return err
	}
	script := filepath.Join(dir, "guard.ps1")
	if err := os.WriteFile(script, GuardScript, 0o600); err != nil {
		return err
	}
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("startup: current user: %w", err)
	}
	return createXML(brand.TaskGuard, GuardTaskXML(Guard{
		PowerShell: filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"),
		Script:     script,
		State:      state,
		UserSID:    tu.User.Sid.String(),
		Log:        filepath.Join(dir, "guard.log"),
	}))
}

func createXML(name, xml string) error {
	f, err := os.CreateTemp("", "ghostline-task-*.xml")
	if err != nil {
		return err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.Write(EncodeUTF16LE(xml)); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	out, err := winutil.HiddenCmd("schtasks", []string{"/Create", "/TN", name, "/XML", filepath.Clean(path), "/F"}, "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("startup: schtasks /Create %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete removes the task; a missing task is not an error.
func Delete(name string) error {
	if !Exists(name) {
		return nil
	}
	out, err := winutil.HiddenCmd("schtasks", []string{"/Delete", "/TN", name, "/F"}, "").CombinedOutput()
	if err != nil {
		return fmt.Errorf("startup: schtasks /Delete %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Exists reports whether the task is registered.
func Exists(name string) bool {
	err := winutil.HiddenCmd("schtasks", []string{"/Query", "/TN", name}, "").Run()
	// A non-zero exit means "no such task"; anything else means schtasks
	// itself could not run, and the answer is a guess.
	if err != nil && !errors.As(err, new(*exec.ExitError)) {
		slog.Warn("startup: schtasks /Query failed to run", "err", err, "task", name)
	}
	return err == nil
}

type taskScheduler struct{ exe, guardDir, state string }

// NewTaskScheduler registers exe's logon tasks with Task Scheduler.
// guardDir holds the network guard script; state is the state.json it
// reads.
func NewTaskScheduler(exe, guardDir, state string) Manager {
	return taskScheduler{exe: exe, guardDir: guardDir, state: state}
}

func (t taskScheduler) SetAutostart(on bool) error {
	if on {
		return Create(AutostartTask(t.exe))
	}
	return Delete(brand.TaskAutostart)
}

// CreateRecovery registers the --restore task and the network guard. The
// guard is a last resort for when an antivirus removes ghostline.exe:
// failing to set it up is logged, not fatal.
func (t taskScheduler) CreateRecovery() error {
	if err := Create(RecoveryTask(t.exe)); err != nil {
		return err
	}
	if err := CreateGuard(t.guardDir, t.state); err != nil {
		slog.Warn("startup: network guard task not created", "err", err)
	}
	return nil
}

func (t taskScheduler) DeleteRecovery() error {
	return errors.Join(Delete(RecoveryTask(t.exe).Name), Delete(brand.TaskGuard))
}
