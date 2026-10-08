package dpi

import (
	"errors"
	"fmt"
)

// nftInterceptor queues packets to nfqws2 through an nftables table. Its
// system calls are fields so the logic is tested without root
// (interceptor_linux.go fills them).
type nftInterceptor struct {
	filter   []CaptureRule
	install  func([]CaptureRule) error
	remove   func() error
	missing  func() []string
	modprobe func(module string) error
	queue    func() ([]byte, error) // /proc/net/netfilter/nfnetlink_queue
	prepDir  func(dir string) error
}

// Prepare loads missing kernel modules, makes the engine directory
// readable by the drop user, and (re)installs the table. A table left by a
// previous run is replaced.
func (i *nftInterceptor) Prepare(dir string) error {
	if miss := i.missing(); len(miss) > 0 {
		var errs []error
		for _, m := range miss {
			errs = append(errs, i.modprobe(m))
		}
		if still := i.missing(); len(still) > 0 {
			return fmt.Errorf("%w: missing %v: %v", ErrKernelUnsupported, still, errors.Join(errs...))
		}
	}
	if err := i.prepDir(dir); err != nil {
		return err
	}
	return i.install(i.filter)
}

func (i *nftInterceptor) Ready(int) bool {
	b, err := i.queue()
	return err == nil && queueBound(b, QueueNum)
}

func (i *nftInterceptor) Cleanup() error { return i.remove() }

func (i *nftInterceptor) Info() InterceptorInfo {
	return InterceptorInfo{Mechanism: fmt.Sprintf("nftables inet %s, queue %d", nftTable, QueueNum)}
}
