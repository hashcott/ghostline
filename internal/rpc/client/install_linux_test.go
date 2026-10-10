package client

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/issue"
	"github.com/hashcott/ghostline/internal/sysinstall"
	"github.com/stretchr/testify/require"
)

type recRun struct {
	calls [][]string
	err   error
}

func (r *recRun) run(name string, args ...string) error {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.err
}

func testInstaller(t *testing.T, env map[string]string, exe string, exists ...string) (*installer, *recRun) {
	t.Helper()
	r := &recRun{}
	return &installer{
		getenv:     func(k string) string { return env[k] },
		exists:     func(p string) bool { _, err := os.Stat(p); return err == nil || contains(exists, p) },
		osRelease:  func() []byte { return []byte("ID=arch\n") },
		executable: func() (string, error) { return exe, nil },
		run:        r.run,
	}, r
}

func TestIssueFields_NameThePackage(t *testing.T) {
	in, _ := testInstaller(t, map[string]string{"APPIMAGE": "/home/u/Ghostline.AppImage"}, "/tmp/.mount/usr/bin/ghostline")
	in.osRelease = func() []byte { return []byte("ID=fedora
PRETTY_NAME=\"Fedora Linux 41\"
") }
	f := in.issueFields()
	require.Equal(t, "Linux", f.OS)
	require.Equal(t, "Fedora Linux 41", f.OSVersion)
	require.Equal(t, issue.PkgAppImage, f.Package)
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if x == y {
			return true
		}
	}
	return false
}

func TestInstaller_Info(t *testing.T) {
	in, _ := testInstaller(t, map[string]string{"APPIMAGE": "/h/G.AppImage"}, "/x", sysinstall.SelfUnit)
	require.Equal(t, app.InstallInfo{Kind: "appimage", Unit: true}, in.info())
}

// Root cannot read the AppImage's FUSE mount: the daemon is copied to the
// runtime directory first.
func TestInstaller_AppImageCopiesThenPkexec(t *testing.T) {
	appdir, runtime := t.TempDir(), t.TempDir()
	src := filepath.Join(appdir, "usr", "lib", "ghostline", "ghostlined")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte("daemon"), 0o755))
	in, r := testInstaller(t, map[string]string{"APPIMAGE": "/h/G.AppImage", "APPDIR": appdir, "XDG_RUNTIME_DIR": runtime}, filepath.Join(appdir, "usr", "bin", "ghostline"))
	require.NoError(t, in.install())
	require.Len(t, r.calls, 1)
	dst := r.calls[0][1]
	require.Equal(t, runtime, filepath.Dir(filepath.Dir(dst)), "in the runtime directory")
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "daemon", string(got))
	fi, _ := os.Stat(dst)
	require.Equal(t, os.FileMode(0o755), fi.Mode().Perm())
	require.Equal(t, [][]string{{"pkexec", dst, "--install-system"}}, r.calls)
}

func TestInstaller_TarballUsesItsDaemon(t *testing.T) {
	root := t.TempDir()
	daemon := filepath.Join(root, "lib", "ghostline", "ghostlined")
	require.NoError(t, os.MkdirAll(filepath.Dir(daemon), 0o755))
	require.NoError(t, os.WriteFile(daemon, []byte("d"), 0o755))
	in, r := testInstaller(t, nil, filepath.Join(root, "bin", "ghostline"))
	require.NoError(t, in.install())
	require.Equal(t, [][]string{{"pkexec", daemon, "--install-system"}}, r.calls)

	in, r = testInstaller(t, nil, filepath.Join(t.TempDir(), "ghostline"))
	err := in.install()
	require.ErrorContains(t, err, app.CodeServiceInstallManual)
	require.Empty(t, r.calls)
}

func TestInstaller_StartService(t *testing.T) {
	in, r := testInstaller(t, nil, "/usr/bin/ghostline", sysinstall.PackageUnit)
	require.NoError(t, in.start())
	require.Equal(t, [][]string{{"pkexec", "systemctl", "enable", "--now", "ghostline.service"}}, r.calls)
}

// Review Focus 5: dismissed, wrong password or no polkit agent.
func TestInstaller_PkexecFailureSaysWhatToRun(t *testing.T) {
	in, r := testInstaller(t, nil, "/usr/bin/ghostline", sysinstall.PackageUnit)
	r.err = &exec.ExitError{}
	err := in.start()
	require.True(t, strings.HasPrefix(err.Error(), app.CodePkexecFailed+": sudo systemctl enable --now ghostline.service"), err.Error())
	r.err = errors.New(`exec: "pkexec": executable file not found in $PATH`)
	require.ErrorContains(t, in.start(), app.CodePkexecFailed)
}

// Review I4: without XDG_RUNTIME_DIR the daemon is staged in a new
// private directory, never a fixed path in /tmp that another local user
// could create first and swap the binary in before pkexec runs it.
func TestInstaller_AppImageWithoutRuntimeDirUsesPrivateTemp(t *testing.T) {
	appdir, tmp := t.TempDir(), t.TempDir()
	t.Setenv("TMPDIR", tmp)
	src := filepath.Join(appdir, "usr", "lib", "ghostline", "ghostlined")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte("daemon"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "ghostline-install"), 0o777), "planted by someone else")
	in, r := testInstaller(t, map[string]string{"APPIMAGE": "/h/G.AppImage", "APPDIR": appdir}, filepath.Join(appdir, "usr", "bin", "ghostline"))
	require.NoError(t, in.install())
	require.Len(t, r.calls, 1)
	dst := r.calls[0][1]
	require.NotEqual(t, filepath.Join(tmp, "ghostline-install", "ghostlined"), dst)
	fi, err := os.Stat(filepath.Dir(dst))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), fi.Mode().Perm())
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "daemon", string(got))
}
