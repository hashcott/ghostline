// Package sysinstall installs Ghostline's background service without a
// package manager (the AppImage and the tar.gz): the daemon is copied to
// SelfDaemon and a systemd unit is written to SelfUnit. The unit template
// is also the source of the unit the packages ship.
package sysinstall

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed ghostline.service.tmpl
var unitTemplate []byte

// Paths of a package install and of a self-install.
const (
	PackageDaemon = "/usr/lib/ghostline/ghostlined"
	PackageUnit   = "/usr/lib/systemd/system/ghostline.service"
	SelfDaemon    = "/var/lib/ghostline/bin/ghostlined"
	SelfUnit      = "/etc/systemd/system/ghostline.service"

	unitName  = "ghostline.service"
	groupName = "ghostline"
	stateDir  = "/var/lib/ghostline"
	logDir    = "/var/log/ghostline"
)

// ErrPackaged refuses a self-install over a package install.
var ErrPackaged = errors.New("sysinstall: Ghostline is installed by a package; update it with the package manager")

// Unit is the systemd unit running daemon.
func Unit(daemon string) []byte {
	return bytes.ReplaceAll(unitTemplate, []byte("@DAEMON@"), []byte(daemon))
}

// System is what an install changes outside the file system.
type System interface {
	Systemctl(args ...string) error
	IsActive(unit string) bool
	GroupExists(name string) bool
	AddGroup(name string) error
}

// Installer installs from Self (the running ghostlined) under Root ("/"
// outside tests).
type Installer struct {
	Root string
	Self string
	Sys  System
}

func (i Installer) path(p string) string { return filepath.Join(i.Root, p) }

// Install copies the daemon, writes the unit, creates the ghostline group
// and starts the service, or restarts it when it runs: running it again
// with a newer binary is the update.
func (i Installer) Install() error {
	if _, err := os.Stat(i.path(PackageUnit)); err == nil {
		return ErrPackaged
	}
	// Read whole first: Self may be the installed copy itself.
	bin, err := os.ReadFile(i.Self)
	if err != nil {
		return err
	}
	// nfqws2 runs as nobody under stateDir/bin: both must be traversable.
	for _, d := range []string{stateDir, filepath.Dir(SelfDaemon)} {
		if err := os.MkdirAll(i.path(d), 0o755); err != nil {
			return err
		}
		if err := os.Chmod(i.path(d), 0o755); err != nil {
			return err
		}
	}
	if err := writeVerified(i.path(SelfDaemon), bin, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(i.path(SelfUnit)), 0o755); err != nil {
		return err
	}
	if err := writeVerified(i.path(SelfUnit), Unit(SelfDaemon), 0o644); err != nil {
		return err
	}
	if !i.Sys.GroupExists(groupName) {
		if err := i.Sys.AddGroup(groupName); err != nil {
			return err
		}
	}
	if err := i.Sys.Systemctl("daemon-reload"); err != nil {
		return err
	}
	if i.Sys.IsActive(unitName) {
		if err := i.Sys.Systemctl("enable", unitName); err != nil {
			return err
		}
		return i.Sys.Systemctl("restart", unitName)
	}
	return i.Sys.Systemctl("enable", "--now", unitName)
}

// Uninstall restores the system (removeAll: what the package's prerm runs),
// stops and removes the service and the installed daemon; purge also
// removes the data and the logs. The ghostline group stays: users may have
// been added to it.
func (i Installer) Uninstall(purge bool, removeAll func() error) error {
	errRestore := removeAll()
	_, err := os.Stat(i.path(SelfUnit))
	hadUnit := err == nil
	if err := i.Sys.Systemctl("disable", "--now", unitName); err != nil && hadUnit {
		return errors.Join(errRestore, err)
	}
	if err := os.Remove(i.path(SelfUnit)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(errRestore, err)
	}
	if hadUnit {
		if err := i.Sys.Systemctl("daemon-reload"); err != nil {
			return errors.Join(errRestore, err)
		}
	}
	remove := []string{filepath.Dir(SelfDaemon)}
	if purge {
		remove = []string{stateDir, logDir}
	}
	for _, d := range remove {
		if err := os.RemoveAll(i.path(d)); err != nil {
			return errors.Join(errRestore, err)
		}
	}
	return errRestore
}

// writeVerified writes data through a temporary file, checks what landed
// on disk, then renames it into place.
func writeVerified(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		os.Remove(tmp)
		return err
	}
	got, err := os.ReadFile(tmp)
	if err != nil || sha256.Sum256(got) != sha256.Sum256(data) {
		os.Remove(tmp)
		return fmt.Errorf("sysinstall: %s did not verify after writing: %v", path, err)
	}
	return os.Rename(tmp, path)
}
