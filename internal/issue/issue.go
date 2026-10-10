// Package issue builds the link to GitHub's bug report form
// (.github/ISSUE_TEMPLATE/bug.yml) with what Ghostline knows about this
// computer already filled in.
package issue

import (
	"bufio"
	"bytes"
	"net/url"
	"strings"
)

// Fields are bug.yml's fields the app can fill; "" leaves a field empty.
// They are text inputs: GitHub fills inputs from the link, not dropdowns.
type Fields struct {
	Version   string // "0.7.0"
	OSVersion string // "Windows 11 23H2, OS Build 22631.4317", "Ubuntu 24.04 LTS"
	Package   string // "Windows installer (.exe)", "Linux AppImage", …
}

// The packages, as the form's description lists them.
const (
	PkgInstaller = "Windows installer (.exe)"
	PkgPortable  = "Windows portable (.zip)"
	PkgDeb       = "Linux .deb"
	PkgRPM       = "Linux .rpm"
	PkgAppImage  = "Linux AppImage"
	PkgTarball   = "Linux tar.gz"
	PkgArch      = "Arch PKGBUILD"
)

// URL is repo's new-issue link for the bug form, prefilled with f.
func URL(repo string, f Fields) string {
	q := url.Values{"template": {"bug.yml"}}
	for k, v := range map[string]string{"version": f.Version, "os-version": f.OSVersion, "package": f.Package} {
		if v != "" {
			q.Set(k, v)
		}
	}
	return strings.TrimSuffix(repo, "/") + "/issues/new?" + q.Encode()
}

// LinuxPackage names the package a Linux install came from. kind is
// sysinstall's ("appimage", "tarball" or "package"); a package is told
// apart by the distribution's family in /etc/os-release.
func LinuxPackage(kind string, osRelease []byte) string {
	switch kind {
	case "appimage":
		return PkgAppImage
	case "tarball":
		return PkgTarball
	case "package":
		ids := strings.Fields(osField(osRelease, "ID") + " " + osField(osRelease, "ID_LIKE"))
		for _, id := range ids {
			switch id {
			case "debian", "ubuntu":
				return PkgDeb
			case "fedora", "rhel", "centos", "suse", "opensuse":
				return PkgRPM
			case "arch":
				return PkgArch
			}
		}
	}
	return ""
}

// LinuxVersion is the distribution's name and version from /etc/os-release.
func LinuxVersion(osRelease []byte) string {
	if v := osField(osRelease, "PRETTY_NAME"); v != "" {
		return v
	}
	return strings.TrimSpace(osField(osRelease, "NAME") + " " + osField(osRelease, "VERSION_ID"))
}

func osField(osRelease []byte, key string) string {
	sc := bufio.NewScanner(bytes.NewReader(osRelease))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), key+"="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}
