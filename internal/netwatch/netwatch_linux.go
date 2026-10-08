package netwatch

import (
	"errors"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// Watch reports link, address and route changes from the kernel
// (NETLINK_ROUTE), debounced by Delay.
func Watch(onChange func()) (func(), error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	groups := uint32(unix.RTMGRP_LINK | unix.RTMGRP_IPV4_IFADDR | unix.RTMGRP_IPV6_IFADDR | unix.RTMGRP_IPV4_ROUTE | unix.RTMGRP_IPV6_ROUTE)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK, Groups: groups}); err != nil {
		unix.Close(fd)
		return nil, err
	}
	// A receive timeout lets the reader notice stop without closing the fd
	// under it.
	tv := unix.Timeval{Usec: 250_000}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		unix.Close(fd)
		return nil, err
	}
	trigger, stopDebounce := Debounce(Delay, onChange)
	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64<<10)
		for !done.Load() {
			if n, _, err := unix.Recvfrom(fd, buf, 0); recvTriggers(n, err) {
				trigger()
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			done.Store(true)
			wg.Wait()
			stopDebounce()
			unix.Close(fd)
		})
	}, nil
}

// recvTriggers reports whether a receive means the network changed: a
// message, or ENOBUFS (the kernel dropped messages, so a change went
// unseen). A timeout (EAGAIN) or an interrupt only lets the reader check
// for stop.
func recvTriggers(n int, err error) bool {
	if err != nil {
		return errors.Is(err, unix.ENOBUFS)
	}
	return n > 0
}
