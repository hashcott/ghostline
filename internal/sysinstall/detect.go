package sysinstall

import (
	"bufio"
	"bytes"
	"strings"
)

// Info is how this copy of Ghostline was installed, for the GUI's service
// buttons.
type Info struct {
	Kind    string // "appimage", "package" or "tarball"
	Unit    bool   // a ghostline.service exists (package or self-install)
	SteamOS bool   // pkexec needs the deck user to have a password first
	// Packaged: the service came from a deb, rpm or Arch package, which
	// --install-system leaves alone.
	Packaged bool
}

// Detect reads the AppImage's environment, the unit files and
// /etc/os-release.
func Detect(getenv func(string) string, exists func(string) bool, osRelease []byte) Info {
	packaged := exists(PackageUnit)
	in := Info{Kind: "tarball", Unit: packaged || exists(SelfUnit), SteamOS: osID(osRelease) == "steamos", Packaged: packaged}
	switch {
	case getenv("APPIMAGE") != "":
		in.Kind = "appimage"
	case packaged:
		in.Kind = "package"
	}
	return in
}

func osID(osRelease []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(osRelease))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "ID="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}
