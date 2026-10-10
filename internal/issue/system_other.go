//go:build !windows

package issue

import "os"

// System is what this computer tells the bug form: the distribution. Which
// package Ghostline came from is the window's to say (rpc/client), not the
// service's.
func System(version string, _ bool) Fields {
	osr, _ := os.ReadFile("/etc/os-release")
	return Fields{Version: version, OSVersion: LinuxVersion(osr)}
}
