package issue

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURL_FillsKnownFields(t *testing.T) {
	u, err := url.Parse(URL("https://github.com/hashcott/ghostline/", Fields{
		Version: "0.7.0", OSVersion: "Windows 10 22H2, OS Build 19045.4529", Package: PkgInstaller,
	}))
	require.NoError(t, err)
	require.Equal(t, "/hashcott/ghostline/issues/new", u.Path)
	q := u.Query()
	require.Equal(t, "bug.yml", q.Get("template"))
	require.Equal(t, "0.7.0", q.Get("version"))
	require.False(t, q.Has("os"), "the form has no OS dropdown: GitHub cannot fill one")
	require.Equal(t, "Windows 10 22H2, OS Build 19045.4529", q.Get("os-version"))
	require.Equal(t, "Windows installer (.exe)", q.Get("package"))
}

func TestURL_LeavesUnknownFieldsOut(t *testing.T) {
	u, err := url.Parse(URL("https://github.com/hashcott/ghostline", Fields{Version: "0.7.0"}))
	require.NoError(t, err)
	q := u.Query()
	require.False(t, q.Has("package"))
	require.False(t, q.Has("os-version"))
}

func TestWindowsVersion(t *testing.T) {
	require.Equal(t, "Windows 10 22H2, OS Build 19045.4529", WindowsVersion(WindowsRelease{Build: 19045, UBR: 4529, DisplayVersion: "22H2"}))
	require.Equal(t, "Windows 11 23H2, OS Build 22631.4317", WindowsVersion(WindowsRelease{Build: 22631, UBR: 4317, DisplayVersion: "23H2"}),
		"Windows 11 builds start at 22000")
	require.Equal(t, "Windows 10 1909, OS Build 18363.1556", WindowsVersion(WindowsRelease{Build: 18363, UBR: 1556, DisplayVersion: "1909"}))
	require.Equal(t, "Windows 10", WindowsVersion(WindowsRelease{}), "nothing read: no build")
}

func TestLinuxPackage(t *testing.T) {
	ubuntu := []byte("NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n")
	fedora := []byte("NAME=\"Fedora Linux\"\nID=fedora\nVERSION_ID=41\n")
	manjaro := []byte("NAME=\"Manjaro Linux\"\nID=manjaro\nID_LIKE=arch\n")
	opensuse := []byte("ID=\"opensuse-tumbleweed\"\nID_LIKE=\"opensuse suse\"\n")
	require.Equal(t, PkgDeb, LinuxPackage("package", ubuntu))
	require.Equal(t, PkgRPM, LinuxPackage("package", fedora))
	require.Equal(t, PkgArch, LinuxPackage("package", manjaro))
	require.Equal(t, PkgRPM, LinuxPackage("package", opensuse))
	require.Equal(t, "", LinuxPackage("package", []byte("ID=nixos\n")), "unknown family: the reporter picks")
	require.Equal(t, PkgAppImage, LinuxPackage("appimage", fedora))
	require.Equal(t, PkgTarball, LinuxPackage("tarball", ubuntu))
}

func TestLinuxVersion(t *testing.T) {
	require.Equal(t, "Ubuntu 24.04.1 LTS", LinuxVersion([]byte("NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n")))
	require.Equal(t, "Fedora Linux 41", LinuxVersion([]byte("NAME=\"Fedora Linux\"\nVERSION_ID=41\n")))
	require.Equal(t, "", LinuxVersion(nil))
}
