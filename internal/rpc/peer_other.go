//go:build !linux

package rpc

import (
	"errors"
	"net"
)

// AllowGroups rejects every peer: the daemon runs only on Linux for now.
func AllowGroups([]string, ...int) func(net.Conn) error {
	return func(net.Conn) error { return errors.New(CodeNotAuthorized + ": no daemon on this OS") }
}
