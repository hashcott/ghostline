package rpc

import (
	"errors"
	"fmt"
	"net"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

// AllowGroups authorises a Unix-socket peer by its credentials
// (SO_PEERCRED): root, a uid in extraUIDs, or a member of groups.
func AllowGroups(groups []string, extraUIDs ...int) func(net.Conn) error {
	return func(c net.Conn) error {
		uc, ok := c.(*net.UnixConn)
		if !ok {
			return errors.New(CodeNotAuthorized + ": not a Unix socket")
		}
		raw, err := uc.SyscallConn()
		if err != nil {
			return fmt.Errorf("%s: %w", CodeNotAuthorized, err)
		}
		var cred *unix.Ucred
		var credErr error
		if err := raw.Control(func(fd uintptr) {
			cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		}); err != nil || credErr != nil {
			return fmt.Errorf("%s: %w", CodeNotAuthorized, errors.Join(err, credErr))
		}
		if allowed(cred.Uid, groupNames(cred.Uid), groups, extraUIDs) {
			return nil
		}
		return fmt.Errorf("%s: uid %d is not in %v", CodeNotAuthorized, cred.Uid, groups)
	}
}

// groupNames lists the names of uid's groups; unknown users have none.
func groupNames(uid uint32) []string {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return nil
	}
	ids, err := u.GroupIds()
	if err != nil {
		return nil
	}
	var names []string
	for _, id := range ids {
		if g, err := user.LookupGroupId(id); err == nil {
			names = append(names, g.Name)
		}
	}
	return names
}
