package sysdns

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/hashcott/ghostline/internal/netwatch"
	"golang.org/x/sys/unix"
)

// fileDebounce settles a burst of writes (tests shorten it).
var fileDebounce = netwatch.Delay

// watchFile reports when path is written, replaced or removed. It watches
// the parent directory, because tools replace resolv.conf by renaming a
// new file over it.
func watchFile(path string, onChange func()) (func(), error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(fd, filepath.Dir(path), unix.IN_CLOSE_WRITE|unix.IN_MOVED_TO|unix.IN_CREATE|unix.IN_DELETE); err != nil {
		unix.Close(fd)
		return nil, err
	}
	name := filepath.Base(path)
	trigger, stopDebounce := netwatch.Debounce(fileDebounce, onChange)
	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 64<<10)
		for !done.Load() {
			// Poll with a timeout so stop is noticed without closing fd
			// under the reader.
			n, err := unix.Poll([]unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}, 250)
			if err != nil || n == 0 {
				continue
			}
			m, err := unix.Read(fd, buf)
			if err != nil || m <= 0 {
				continue
			}
			for off := 0; off+unix.SizeofInotifyEvent <= m; {
				ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[off]))
				nameLen := int(ev.Len)
				evName := ""
				if nameLen > 0 {
					raw := buf[off+unix.SizeofInotifyEvent : off+unix.SizeofInotifyEvent+nameLen]
					for i, c := range raw {
						if c == 0 {
							raw = raw[:i]
							break
						}
					}
					evName = string(raw)
				}
				if evName == name {
					trigger()
				}
				off += unix.SizeofInotifyEvent + nameLen
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
