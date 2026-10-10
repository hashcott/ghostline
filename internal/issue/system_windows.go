package issue

import (
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// System is what this computer tells the bug form: the Windows release,
// and the installer or the portable build.
func System(version string, portable bool) Fields {
	f := Fields{Version: version, Package: PkgInstaller}
	if portable {
		f.Package = PkgPortable
	}
	f.OSVersion = WindowsVersion(ThisWindows())
	return f
}

// ThisWindows reads this computer's Windows release from the registry; a
// value it cannot read is left zero.
func ThisWindows() WindowsRelease {
	var r WindowsRelease
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return r
	}
	defer k.Close()
	if s, _, err := k.GetStringValue("CurrentBuild"); err == nil {
		r.Build, _ = strconv.Atoi(s)
	}
	if n, _, err := k.GetIntegerValue("UBR"); err == nil {
		r.UBR = int(n)
	}
	if s, _, err := k.GetStringValue("DisplayVersion"); err == nil && s != "" {
		r.DisplayVersion = s
	} else if s, _, err := k.GetStringValue("ReleaseId"); err == nil {
		r.DisplayVersion = s
	}
	return r
}
