package client

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/issue"
	"github.com/hashcott/ghostline/internal/sysinstall"
)

// installer runs the service buttons: pkexec of ghostlined --install-system
// or of systemctl. Its system calls are fields for tests.
type installer struct {
	getenv     func(string) string
	exists     func(string) bool
	osRelease  func() []byte
	executable func() (string, error)
	run        func(name string, args ...string) error
}

func newInstaller() *installer {
	return &installer{
		getenv:     os.Getenv,
		exists:     func(p string) bool { _, err := os.Stat(p); return err == nil },
		osRelease:  func() []byte { b, _ := os.ReadFile("/etc/os-release"); return b },
		executable: os.Executable,
		run:        func(name string, args ...string) error { return exec.Command(name, args...).Run() },
	}
}

func (in *installer) info() app.InstallInfo {
	d := sysinstall.Detect(in.getenv, in.exists, in.osRelease())
	return app.InstallInfo{Kind: d.Kind, Unit: d.Unit, SteamOS: d.SteamOS, Packaged: d.Packaged}
}

// issueFields is this install as the bug form names it.
func (in *installer) issueFields() issue.Fields {
	osr := in.osRelease()
	return issue.Fields{Version: brand.Version, OSVersion: issue.LinuxVersion(osr),
		Package: issue.LinuxPackage(in.info().Kind, osr)}
}

// install runs this build's ghostlined --install-system as root.
func (in *installer) install() error {
	var daemon string
	switch in.info().Kind {
	case "appimage":
		// Root cannot read the AppImage's FUSE mount: copy the daemon out,
		// into a new private (0700) directory so no other local user can
		// swap the binary before pkexec runs it as root.
		base := in.getenv("XDG_RUNTIME_DIR")
		if base == "" {
			base = os.TempDir()
		}
		dir, err := os.MkdirTemp(base, "ghostline-install-*")
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, "ghostlined")
		if err := copyExecutable(filepath.Join(in.getenv("APPDIR"), "usr", "lib", "ghostline", "ghostlined"), dst); err != nil {
			return err
		}
		daemon = dst
	case "tarball":
		exe, err := in.executable()
		if err != nil {
			return err
		}
		// install.sh puts the GUI in <prefix>/bin and the daemon in
		// <prefix>/lib/ghostline.
		daemon = filepath.Join(filepath.Dir(filepath.Dir(exe)), "lib", "ghostline", "ghostlined")
		if !in.exists(daemon) {
			return &app.AppError{Code: app.CodeServiceInstallManual}
		}
	default:
		return &app.AppError{Code: app.CodeServiceActionUnsupported}
	}
	return in.pkexec(daemon, "--install-system")
}

// start enables and starts the installed service.
func (in *installer) start() error {
	return in.pkexec("systemctl", "enable", "--now", "ghostline.service")
}

// pkexec runs a command as root. On failure (dismissed, wrong password,
// no polkit agent) the error carries the command to run in a terminal.
func (in *installer) pkexec(args ...string) error {
	if err := in.run("pkexec", args...); err != nil {
		return fmt.Errorf("%s: sudo %s", app.CodePkexecFailed, strings.Join(args, " "))
	}
	return nil
}

func copyExecutable(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}
