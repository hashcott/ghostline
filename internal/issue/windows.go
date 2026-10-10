package issue

import "fmt"

// WindowsRelease is what HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion
// says about this Windows.
type WindowsRelease struct {
	Build          int    // CurrentBuild: 19045
	UBR            int    // update build revision: 4529
	DisplayVersion string // "22H2"; ReleaseId ("1909") on Windows before 20H2
}

// WindowsVersion is the form's OS line for r. Windows 11 still calls itself
// Windows 10 in ProductName; its builds start at 22000.
func WindowsVersion(r WindowsRelease) string {
	version := "Windows 10"
	if r.Build >= 22000 {
		version = "Windows 11"
	}
	if r.DisplayVersion != "" {
		version += " " + r.DisplayVersion
	}
	if r.Build > 0 {
		version += fmt.Sprintf(", OS Build %d.%d", r.Build, r.UBR)
	}
	return version
}
