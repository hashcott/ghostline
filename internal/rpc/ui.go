package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// UITimeout bounds a reverse ui call: a dialog may stay open a while.
const UITimeout = 10 * time.Minute

// ErrNoUI means no GUI can answer: the call did not come from one, or it
// went away.
var ErrNoUI = errors.New(CodeNoUI)

// CallUI asks the client that made the current call (ctx is the call's) to
// run kind, e.g. a file dialog, and decodes its answer into result (nil to
// drop it).
func CallUI(ctx context.Context, kind string, result any, args ...any) error {
	sc, _ := ctx.Value(connKey{}).(*sconn)
	if sc == nil {
		return ErrNoUI
	}
	raw, err := encodeArgs(args)
	if err != nil {
		return err
	}
	ch := make(chan Msg, 1)
	sc.mu.Lock()
	sc.nextUID++
	uid := sc.nextUID
	sc.waiting[uid] = ch
	sc.mu.Unlock()
	forget := func() {
		sc.mu.Lock()
		delete(sc.waiting, uid)
		sc.mu.Unlock()
	}
	if err := sc.send(Msg{UID: uid, UI: kind, Args: raw}); err != nil {
		forget()
		return fmt.Errorf("%w: %v", ErrNoUI, err)
	}
	timer := time.NewTimer(UITimeout)
	defer timer.Stop()
	select {
	case m := <-ch:
		if m.Error != nil {
			return m.Error.remote()
		}
		if result != nil && len(m.Result) > 0 && string(m.Result) != "null" {
			return json.Unmarshal(m.Result, result)
		}
		return nil
	case <-sc.gone:
		forget()
		return ErrNoUI
	case <-timer.C:
		forget()
		return fmt.Errorf("%w: %w", ErrNoUI, context.DeadlineExceeded)
	case <-ctx.Done():
		forget()
		return ctx.Err()
	}
}
