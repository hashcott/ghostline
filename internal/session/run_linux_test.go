package session

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// self runs agents as the test binary, under the current user.
func self(t *testing.T) (*linuxSessions, User) {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	s := &linuxSessions{exe: exe, extraEnv: []string{"GHOSTLINE_TEST_AGENT=1"}}
	return s, User{UID: os.Getuid(), GID: os.Getgid(), Name: "me", Home: t.TempDir(), Desktop: "KDE"}
}

func TestRun_SpawnsAgentAndReturns(t *testing.T) {
	s, u := self(t)
	var out map[string]int
	require.NoError(t, s.Run(u, "echo", map[string]int{"x": 1}, &out))
	require.Equal(t, map[string]int{"x": 1}, out)
}

func TestRun_AgentErrorIsErrAgent(t *testing.T) {
	s, u := self(t)
	err := s.Run(u, "fail", nil, nil)
	require.ErrorIs(t, err, ErrAgent)
	require.Contains(t, err.Error(), "BOOM")
}

// The agent gets the user's session environment and nothing of the
// daemon's own.
func TestRun_EnvIsTheSessionOnly(t *testing.T) {
	t.Setenv("LEAK_ME", "1")
	s, u := self(t)
	var env []string
	require.NoError(t, s.Run(u, "env", nil, &env))
	for _, want := range []string{
		"HOME=" + u.Home,
		fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", u.UID),
		fmt.Sprintf("DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/%d/bus", u.UID),
		"XDG_CURRENT_DESKTOP=KDE",
		"USER=me",
	} {
		require.Contains(t, env, want)
	}
	require.False(t, slices.ContainsFunc(env, func(e string) bool { return e == "LEAK_ME=1" }))
}

func TestStream_LinesUntilStop(t *testing.T) {
	s, u := self(t)
	var mu sync.Mutex
	n := 0
	stop, err := s.Stream(u, "tick", nil, func(json.RawMessage) { mu.Lock(); n++; mu.Unlock() })
	require.NoError(t, err)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return n >= 3 }, 3*time.Second, 10*time.Millisecond)
	stop()
	mu.Lock()
	after := n
	mu.Unlock()
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, after, n, "no lines after stop")
}
