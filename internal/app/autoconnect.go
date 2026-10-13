package app

import (
	"context"
	"errors"
	"time"
)

// AutoConnectDelays are the waits between connect attempts at logon, when
// the network is often not up yet (about 3 minutes in all).
var AutoConnectDelays = []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second, 60 * time.Second}

// AutoConnect connects at logon. A NO_SERVERS failure there usually means
// the network is not up yet, so it tries again after each of delays while
// Ghostline still shows that error. Cancel (or Disconnect) stops the wait.
func (o *Orchestrator) AutoConnect(ctx context.Context, delays []time.Duration) error {
	err := o.Connect(ctx)
	for _, d := range delays {
		var ae *AppError
		if err == nil || !errors.As(err, &ae) || ae.Code != CodeNoServers {
			return err
		}
		if err := o.waitRetry(ctx, d); err != nil {
			return err
		}
		if !o.showsNoServers() {
			return nil // the user took over
		}
		err = o.Connect(ctx)
	}
	return err
}

func (o *Orchestrator) showsNoServers() bool {
	s := o.Snapshot()
	return s.Status == StatusError && s.Error != nil && s.Error.Code == CodeNoServers
}

// waitRetry sleeps d; Cancel ends the sleep early, as it ends a Connect.
func (o *Orchestrator) waitRetry(ctx context.Context, d time.Duration) error {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	o.mu.Lock()
	if o.cancel != nil {
		o.mu.Unlock()
		return nil // a Connect the user started is running
	}
	o.cancel = cancel
	o.mu.Unlock()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-wctx.Done():
	}
	o.mu.Lock()
	o.cancel = nil
	o.mu.Unlock()
	return wctx.Err()
}
