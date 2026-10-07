package rpc

import "slices"

// DefaultGroups may control the daemon: the usual admin groups of Ubuntu
// (sudo, admin), Fedora, Arch and SteamOS (wheel), and the group the
// packages create for everyone else (ghostline).
var DefaultGroups = []string{"wheel", "sudo", "admin", "ghostline"}

// allowed decides a peer: root, a uid in extra, or a member of any of
// allowedGroups.
func allowed(uid uint32, groupNames, allowedGroups []string, extra []int) bool {
	if uid == 0 || slices.Contains(extra, int(uid)) {
		return true
	}
	for _, g := range groupNames {
		if slices.Contains(allowedGroups, g) {
			return true
		}
	}
	return false
}
