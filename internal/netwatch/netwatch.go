// Package netwatch reports network changes (links, addresses, routes) so
// Ghostline can re-check the system's DNS, debounced like on Windows.
package netwatch

import (
	"errors"
	"sync"
	"time"
)

// Delay is how long a burst of changes settles before onChange runs.
const Delay = 2 * time.Second

// WatchFunc starts watching; stop ends it.
type WatchFunc func(onChange func()) (stop func(), err error)

// Debounce returns trigger, which calls f once d after the last trigger in
// a burst, and stop, which cancels a pending call.
func Debounce(d time.Duration, f func()) (trigger func(), stop func()) {
	var mu sync.Mutex
	var t *time.Timer
	trigger = func() {
		mu.Lock()
		defer mu.Unlock()
		if t != nil {
			t.Stop()
		}
		t = time.AfterFunc(d, f)
	}
	stop = func() {
		mu.Lock()
		defer mu.Unlock()
		if t != nil {
			t.Stop()
		}
	}
	return trigger, stop
}

// Combine starts every watch on the same callback; if one fails, the ones
// already started are stopped.
func Combine(watches ...WatchFunc) WatchFunc {
	return func(onChange func()) (func(), error) {
		var stops []func()
		stopAll := func() {
			for _, s := range stops {
				s()
			}
		}
		for _, w := range watches {
			stop, err := w(onChange)
			if err != nil {
				stopAll()
				return nil, err
			}
			if stop == nil {
				stopAll()
				return nil, errors.New("netwatch: watch without stop")
			}
			stops = append(stops, stop)
		}
		return stopAll, nil
	}
}
