package procs

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLinux_SelfIsAliveWithItsStartTime(t *testing.T) {
	in := NewLinux()
	pid := uint32(os.Getpid())
	st, err := in.StartTime(pid)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), st, 24*time.Hour)
	require.True(t, in.Alive(pid, st))
	require.False(t, in.Alive(pid, st.Add(time.Hour)), "a reused PID must not count")
	require.False(t, in.Alive(999999999, st))
}

func TestLinux_WaitForExitReturnsWhenChildExits(t *testing.T) {
	cmd := exec.Command("true")
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- NewLinux().WaitForExit(uint32(cmd.Process.Pid)) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForExit did not return")
	}
	_ = cmd.Wait()
}

func TestLinux_IsAdminMatchesEuid(t *testing.T) {
	require.Equal(t, os.Geteuid() == 0, NewLinux().IsAdmin())
}
