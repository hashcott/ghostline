package dpi

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

// recIC records the calls a Manager makes to its Interceptor.
type recIC struct {
	c          *calls
	ready      bool
	prepareErr error
}

func (i *recIC) Prepare(string) error { i.c.log = append(i.c.log, "prepare"); return i.prepareErr }
func (i *recIC) Ready(pid int) bool {
	i.c.log = append(i.c.log, fmt.Sprintf("ready:%d", pid))
	return i.ready
}
func (i *recIC) Cleanup() error        { i.c.log = append(i.c.log, "cleanup"); return nil }
func (i *recIC) Info() InterceptorInfo { return InterceptorInfo{Mechanism: "rec"} }

func icRig(t *testing.T, ic *recIC) (*Manager, *fakeRunner) {
	z := &fakeEngine{id: "zapret2", files: map[string]string{"zapret2.exe": sha("z")}}
	r := &fakeRunner{c: ic.c}
	m := NewManager(t.TempDir(), []Installed{{Engine: z, Assets: fstest.MapFS{"zapret2.exe": {Data: []byte("z")}}}}, r, ic, func(time.Duration) {})
	return m, r
}

func TestManager_StartCallsPrepareThenWaitsReady(t *testing.T) {
	ic := &recIC{c: &calls{}, ready: true}
	m, _ := icRig(t, ic)
	pid, err := m.Start(context.Background(), "zapret2", Plan{Strategy: "x"})
	require.NoError(t, err)
	require.Equal(t, 100, pid)
	require.Equal(t, []string{"cleanup", "prepare", "run:zapret2.exe", "ready:100"}, ic.c.log)
	require.Equal(t, "rec", m.Info().Mechanism)
}

func TestManager_NotReadyKillsAndFails(t *testing.T) {
	ic := &recIC{c: &calls{}}
	m, r := icRig(t, ic)
	_, err := m.Start(context.Background(), "zapret2", Plan{Strategy: "x"})
	require.ErrorIs(t, err, ErrStartFailed)
	require.True(t, r.procs[0].killed)
	require.False(t, m.Running())
	// Root run: a failed start left the nftables table behind.
	require.Equal(t, "cleanup", ic.c.log[len(ic.c.log)-1], "a failed start removes the capture it prepared")
}

func TestManager_StopKillsThenCleansUp(t *testing.T) {
	ic := &recIC{c: &calls{}, ready: true}
	m, r := icRig(t, ic)
	_, err := m.Start(context.Background(), "zapret2", Plan{Strategy: "x"})
	require.NoError(t, err)
	ic.c.log = nil
	require.NoError(t, m.Stop())
	require.True(t, r.procs[0].killed)
	require.Equal(t, []string{"cleanup"}, ic.c.log)
}

func TestManager_PrepareErrorIsReturned(t *testing.T) {
	boom := errors.New("boom")
	ic := &recIC{c: &calls{}, ready: true, prepareErr: boom}
	m, _ := icRig(t, ic)
	_, err := m.Start(context.Background(), "zapret2", Plan{Strategy: "x"})
	require.ErrorIs(t, err, boom)
	require.NotContains(t, ic.c.log, "run:zapret2.exe")
}

func TestManager_EnginesInOrder(t *testing.T) {
	rg := newRig(t)
	var ids []string
	for _, e := range rg.m.Engines() {
		ids = append(ids, e.ID())
	}
	require.Equal(t, []string{"goodbyedpi", "zapret2"}, ids)
}
