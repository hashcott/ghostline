package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var quick = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

func TestAutoConnect_RetriesWhileNoServers(t *testing.T) {
	h := newHarness(t)
	h.pick.err, h.pick.errN = &NoServersError{Checked: 727}, 2
	require.NoError(t, h.o.AutoConnect(context.Background(), quick))
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
}

func TestAutoConnect_GivesUpAfterLastDelay(t *testing.T) {
	h := newHarness(t)
	h.pick.err = &NoServersError{Checked: 727}
	require.Error(t, h.o.AutoConnect(context.Background(), quick))
	sn := h.o.Snapshot()
	require.Equal(t, StatusError, sn.Status)
	require.Equal(t, CodeNoServers, sn.Error.Code)
	require.Equal(t, len(quick)+1, countCalls(h.r.list(), "pick"))
}

func TestAutoConnect_OtherErrorsAreNotRetried(t *testing.T) {
	h := newHarness(t)
	h.r.fail["engine.start"] = true
	require.Error(t, h.o.AutoConnect(context.Background(), quick))
	require.Equal(t, 1, countCalls(h.r.list(), "pick"))
}

func TestAutoConnect_StopsWhenTheUserDisconnects(t *testing.T) {
	h := newHarness(t)
	h.pick.err = &NoServersError{Checked: 727}
	done := make(chan error, 1)
	go func() { done <- h.o.AutoConnect(context.Background(), []time.Duration{time.Hour}) }()
	require.Eventually(t, func() bool { return h.o.Snapshot().Status == StatusError }, time.Second, time.Millisecond)
	require.NoError(t, h.o.Disconnect(context.Background()))
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("AutoConnect kept waiting after the user disconnected")
	}
	require.Equal(t, 1, countCalls(h.r.list(), "pick"))
}
