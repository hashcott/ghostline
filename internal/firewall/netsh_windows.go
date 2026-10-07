package firewall

import (
	"path/filepath"

	"github.com/hashcott/ghostline/internal/winutil"
	"golang.org/x/sys/windows"
)

// NewNetsh manages rules for exe through system32\netsh.exe.
func NewNetsh(exe string) *Netsh {
	return &Netsh{Exe: exe, Run: netsh, Public: currentNetworkIsPublic}
}

func system32(name string) string {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return name
	}
	return filepath.Join(dir, name)
}

func netsh(args []string) ([]byte, error) {
	return winutil.HiddenCmd(system32("netsh.exe"), args, "").CombinedOutput()
}

// currentNetworkIsPublic reports whether a connected network uses the
// Public firewall profile (LAN devices cannot reach the proxy then).
func currentNetworkIsPublic() (bool, error) {
	ps := filepath.Join(system32(""), `WindowsPowerShell\v1.0\powershell.exe`)
	out, err := winutil.HiddenCmd(ps, []string{"-NoProfile", "-NonInteractive", "-Command",
		"Get-NetConnectionProfile | ForEach-Object { $_.NetworkCategory.ToString() }"}, "").Output()
	if err != nil {
		return false, err
	}
	return parsePublic(string(out)), nil
}
