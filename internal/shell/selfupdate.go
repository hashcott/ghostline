package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// installerRunner returns core's RunInstaller for a build installed by the
// Windows installer (uninstall.exe next to the exe), or nil: a portable
// copy or a package-managed install updates the way it was installed.
// The installer runs silently with /UPDATE, waits for Ghostline to quit,
// replaces it and opens it again.
func installerRunner(exe string, portable bool, quit func()) func(path string) error {
	if portable {
		return nil
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "uninstall.exe")); err != nil {
		return nil
	}
	return func(path string) error {
		cmd := exec.Command(path, "/S", "/UPDATE")
		if err := cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Process.Release()
		// Let the UI call that asked for the update return first.
		time.AfterFunc(500*time.Millisecond, quit)
		return nil
	}
}
