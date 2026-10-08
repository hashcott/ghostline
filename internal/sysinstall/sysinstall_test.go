package sysinstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSys records systemctl calls and answers is-active and group lookups.
type fakeSys struct {
	calls      []string
	active     bool
	group      bool
	disableErr error
}

func (f *fakeSys) Systemctl(args ...string) error {
	f.calls = append(f.calls, "systemctl "+strings.Join(args, " "))
	if args[0] == "disable" {
		return f.disableErr
	}
	return nil
}
func (f *fakeSys) IsActive(string) bool    { return f.active }
func (f *fakeSys) GroupExists(string) bool { return f.group }
func (f *fakeSys) AddGroup(name string) error {
	f.calls = append(f.calls, "groupadd "+name)
	f.group = true
	return nil
}

var daemonBytes = []byte("\x7fELF ghostlined build")

func newInstaller(t *testing.T) (Installer, *fakeSys) {
	t.Helper()
	root := t.TempDir()
	self := filepath.Join(t.TempDir(), "ghostlined")
	require.NoError(t, os.WriteFile(self, daemonBytes, 0o755))
	sys := &fakeSys{}
	return Installer{Root: root, Self: self, Sys: sys}, sys
}

func at(i Installer, p string) string { return filepath.Join(i.Root, p) }

func TestUnit_FillsDaemonPath(t *testing.T) {
	u := string(Unit("/x/ghostlined"))
	require.Contains(t, u, "ExecStart=/x/ghostlined --daemon\n")
	require.Contains(t, u, "ExecStopPost=/x/ghostlined --restore\n")
	require.NotContains(t, u, "@DAEMON@")
	require.Contains(t, u, "NoNewPrivileges=yes\n")
	require.Contains(t, u, "CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE CAP_SETUID CAP_SETGID CAP_KILL CAP_DAC_OVERRIDE CAP_CHOWN CAP_FOWNER\n")
}

func TestInstall_Fresh(t *testing.T) {
	i, sys := newInstaller(t)
	require.NoError(t, i.Install())
	got, err := os.ReadFile(at(i, SelfDaemon))
	require.NoError(t, err)
	require.Equal(t, daemonBytes, got)
	for p, mode := range map[string]os.FileMode{SelfDaemon: 0o755, "/var/lib/ghostline": 0o755, "/var/lib/ghostline/bin": 0o755} {
		fi, err := os.Stat(at(i, p))
		require.NoError(t, err)
		require.Equal(t, mode, fi.Mode().Perm(), p)
	}
	unit, err := os.ReadFile(at(i, SelfUnit))
	require.NoError(t, err)
	require.Equal(t, Unit(SelfDaemon), unit)
	require.Equal(t, []string{"groupadd ghostline", "systemctl daemon-reload", "systemctl enable --now ghostline.service"}, sys.calls)
	_, err = os.Stat(at(i, SelfDaemon) + ".new")
	require.True(t, os.IsNotExist(err), "no .new left")
}

// Running it again is the update: the new binary is in place and the
// running service restarts onto it.
func TestInstall_AgainRestarts(t *testing.T) {
	i, sys := newInstaller(t)
	sys.active, sys.group = true, true
	require.NoError(t, i.Install())
	require.Equal(t, []string{"systemctl daemon-reload", "systemctl enable ghostline.service", "systemctl restart ghostline.service"}, sys.calls)
}

// Review Focus 2: the installed copy installing itself.
func TestInstall_FromInstalledCopy(t *testing.T) {
	i, _ := newInstaller(t)
	require.NoError(t, i.Install())
	i.Self = at(i, SelfDaemon)
	require.NoError(t, i.Install())
	got, err := os.ReadFile(at(i, SelfDaemon))
	require.NoError(t, err)
	require.Equal(t, daemonBytes, got)
}

func TestInstall_RefusesPackaged(t *testing.T) {
	i, sys := newInstaller(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(at(i, PackageUnit)), 0o755))
	require.NoError(t, os.WriteFile(at(i, PackageUnit), []byte("[Unit]\n"), 0o644))
	require.ErrorIs(t, i.Install(), ErrPackaged)
	_, err := os.Stat(at(i, SelfDaemon))
	require.True(t, os.IsNotExist(err))
	require.Empty(t, sys.calls)
}

func TestUninstall_RemovesServiceKeepsData(t *testing.T) {
	i, sys := newInstaller(t)
	require.NoError(t, i.Install())
	require.NoError(t, os.MkdirAll(at(i, "/var/lib/ghostline/data"), 0o700))
	require.NoError(t, os.WriteFile(at(i, "/var/lib/ghostline/data/x"), nil, 0o600))
	sys.calls = nil
	// The service stops first: it disconnects and its ExecStopPost
	// restores; --remove-certs then takes away what is left.
	removeAll := func() error { sys.calls = append(sys.calls, "remove-certs"); return nil }
	require.NoError(t, i.Uninstall(false, removeAll))
	require.Equal(t, []string{"systemctl disable --now ghostline.service", "remove-certs", "systemctl daemon-reload"}, sys.calls)
	for _, p := range []string{SelfUnit, "/var/lib/ghostline/bin"} {
		_, err := os.Stat(at(i, p))
		require.True(t, os.IsNotExist(err), p)
	}
	_, err := os.Stat(at(i, "/var/lib/ghostline/data/x"))
	require.NoError(t, err, "data stays without --purge")
}

func TestUninstall_Purge(t *testing.T) {
	i, _ := newInstaller(t)
	require.NoError(t, i.Install())
	require.NoError(t, os.MkdirAll(at(i, "/var/log/ghostline"), 0o755))
	require.NoError(t, i.Uninstall(true, func() error { return nil }))
	for _, p := range []string{"/var/lib/ghostline", "/var/log/ghostline"} {
		_, err := os.Stat(at(i, p))
		require.True(t, os.IsNotExist(err), p)
	}
}

// Review Focus 3: nothing installed, systemctl knows no such unit.
func TestUninstall_NothingInstalled(t *testing.T) {
	i, sys := newInstaller(t)
	sys.disableErr = errors.New("Unit file ghostline.service does not exist")
	require.NoError(t, i.Uninstall(false, func() error { return nil }))
}

func TestUninstall_RemoveAllErrorStillUninstalls(t *testing.T) {
	i, _ := newInstaller(t)
	require.NoError(t, i.Install())
	boom := errors.New("restore failed")
	require.ErrorIs(t, i.Uninstall(false, func() error { return boom }), boom)
	_, err := os.Stat(at(i, SelfUnit))
	require.True(t, os.IsNotExist(err))
}
