package dpi

import (
	"errors"
	"log/slog"
)

// driverService is the WinDivert 2.x service name. WinDivert 1.x (shipped
// with GoodbyeDPI 0.2.2) used a versioned name ("WinDivert1.4"), so cleanup
// looks for the prefix.
const driverService = "WinDivert"

// Services controls Windows services.
type Services interface {
	Find(prefix string) ([]string, error)
	Running(name string) (bool, error)
	// Active reports whether a service exists and is not fully stopped
	// (running, paused, or mid-transition). A missing service is not active.
	Active(name string) (bool, error)
	Stop(name string) error
	Delete(name string) error
}

// winDivert is the Interceptor for WinDivert. The engine loads the driver
// itself; Ghostline only removes services left behind and checks that the
// driver came up.
type winDivert struct{ svc Services }

// NewWinDivert captures through the WinDivert driver service.
func NewWinDivert(s Services) Interceptor { return winDivert{svc: s} }

// Prepare checks the driver is free: Cleanup already ran when the previous
// engine stopped (Manager.Start always stops first), so a WinDivert service
// still active is held by another DPI tool the user runs outside Ghostline
// (a standalone GoodbyeDPI or zapret). An in-use kernel driver cannot be
// claimed: the user is asked to close it instead of a cryptic failure.
func (w winDivert) Prepare(string) error {
	names, err := w.svc.Find(driverService)
	if err != nil {
		slog.Warn("dpi: list WinDivert services failed", "err", err)
	}
	for _, n := range names {
		ok, err := w.svc.Active(n)
		if err != nil {
			slog.Warn("dpi: query WinDivert service state failed", "err", err, "service", n)
		}
		if ok {
			slog.Warn("dpi: WinDivert service still active after cleanup; held by another program", "service", n)
			return ErrDriverInUse
		}
	}
	return nil
}

func (w winDivert) Ready(int) bool {
	ok, err := w.svc.Running(driverService)
	if err != nil {
		slog.Warn("dpi: query WinDivert driver state failed", "err", err, "service", driverService)
	}
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
