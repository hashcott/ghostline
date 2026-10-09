package sysinstall

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	none := func(string) bool { return false }
	only := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, x := range paths {
				if p == x {
					return true
				}
			}
			return false
		}
	}
	env := func(kv map[string]string) func(string) string { return func(k string) string { return kv[k] } }
	cases := []struct {
		name   string
		getenv func(string) string
		exists func(string) bool
		osrel  string
		want   Info
	}{
		{"appimage", env(map[string]string{"APPIMAGE": "/h/G.AppImage"}), none, "ID=arch\n", Info{Kind: "appimage"}},
		{"appimage with service", env(map[string]string{"APPIMAGE": "/h/G.AppImage"}), only(SelfUnit), "", Info{Kind: "appimage", Unit: true}},
		{"package", env(nil), only(PackageUnit), "ID=ubuntu\n", Info{Kind: "package", Unit: true, Packaged: true}},
		{"appimage over a package", env(map[string]string{"APPIMAGE": "/h/G.AppImage"}), only(PackageUnit), "", Info{Kind: "appimage", Unit: true, Packaged: true}},
		{"tarball", env(nil), none, "", Info{Kind: "tarball"}},
		{"tarball installed", env(nil), only(SelfUnit), "", Info{Kind: "tarball", Unit: true}},
		{"steamos", env(map[string]string{"APPIMAGE": "/h/G.AppImage"}), none, "NAME=\"SteamOS\"\nID=steamos\nID_LIKE=arch\n", Info{Kind: "appimage", SteamOS: true}},
		{"steamos quoted", env(nil), none, "ID=\"steamos\"\n", Info{Kind: "tarball", SteamOS: true}},
		{"like steamos is not steamos", env(nil), none, "ID=holo\nID_LIKE=steamos\n", Info{Kind: "tarball"}},
	}
	for _, c := range cases {
		require.Equal(t, c.want, Detect(c.getenv, c.exists, []byte(c.osrel)), c.name)
	}
}
