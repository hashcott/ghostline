package shell

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/store"
)

// autostartDir is the user's XDG autostart directory.
func autostartDir(getenv func(string) string) string {
	if d := getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "autostart")
	}
	return filepath.Join(getenv("HOME"), ".config", "autostart")
}

// autostartExe is what the autostart entry starts: the AppImage itself
// when running from one (its mount point changes every run), else this
// executable.
func autostartExe(getenv func(string) string, executable func() (string, error)) (string, error) {
	if p := getenv("APPIMAGE"); p != "" {
		return p, nil
	}
	return executable()
}

// desktopQuote quotes an Exec argument per the Desktop Entry spec.
func desktopQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(s) + `"`
}

func autostartEntry(exe string) []byte {
	return []byte("[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Ghostline\n" +
		"Comment=Encrypted DNS and DPI bypass\n" +
		"Exec=" + desktopQuote(exe) + " --autostart\n" +
		"Icon=ghostline\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n")
}

// syncAutostart makes dir/ghostline.desktop match "start with the system":
// written (only when its content differs) when on, removed when off.
func syncAutostart(dir, exe string, on bool) error {
	path := filepath.Join(dir, "ghostline.desktop")
	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	want := autostartEntry(exe)
	if got, err := os.ReadFile(path); err == nil && bytes.Equal(got, want) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, want, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// applyAutostart syncs the entry with s. Settings with no version did not
// come from the daemon (it was unreachable): they say nothing about the
// user's choice, so the entry is left as it is.
func applyAutostart(s store.Settings, getenv func(string) string, executable func() (string, error)) error {
	if s.Version == 0 {
		return nil
	}
	exe, err := autostartExe(getenv, executable)
	if err != nil {
		return err
	}
	return syncAutostart(autostartDir(getenv), exe, s.StartWithWindows)
}
