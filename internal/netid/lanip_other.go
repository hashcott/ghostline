//go:build !linux

package netid

import "net"

// isOnLAN keeps every interface: no check for container or VM bridges
// here.
func isOnLAN(net.Interface) bool { return true }
