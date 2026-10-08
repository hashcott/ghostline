package dpi

import "errors"

// driverService is the WinDivert 2.x service name. WinDivert 1.x (shipped
// with GoodbyeDPI 0.2.2) used a versioned name ("WinDivert1.4"), so cleanup
// looks for the prefix.
const driverService = "WinDivert"

// Services controls Windows services.
type Services interface {
	Find(prefix string) ([]string, error)
	Running(name string) (bool, error)
	Stop(name string) error
	Delete(name string) error
}

// winDivert is the Interceptor for WinDivert. The engine loads the driver
// itself; Ghostline only removes services left behind and checks that the
// driver came up.
type winDivert struct{ svc Services }

// NewWinDivert captures through the WinDivert driver service.
func NewWinDivert(s Services) Interceptor { return winDivert{svc: s} }

// Prepare has nothing to do: Cleanup already ran when the previous engine
// stopped (Manager.Start always stops first).
func (winDivert) Prepare(string) error { return nil }

func (w winDivert) Ready(int) bool {
	ok, _ := w.svc.Running(driverService)
	return ok
}

// Cleanup stops and deletes every WinDivert service.
func (w winDivert) Cleanup() error {
	names, err := w.svc.Find(driverService)
	errs := []error{err}
	for _, n := range names {
		errs = append(errs, w.svc.Stop(n), w.svc.Delete(n))
	}
	return errors.Join(errs...)
}

func (winDivert) Info() InterceptorInfo {
	return InterceptorInfo{Mechanism: "WinDivert", AVExclusions: true}
}
