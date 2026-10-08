package dpi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLinuxRunner_KillEndsChild(t *testing.T) {
	p, err := NewLinuxRunner().Start("/bin/sleep", []string{"30"}, t.TempDir())
	require.NoError(t, err)
	require.False(t, p.Exited())
	require.NoError(t, p.Kill())
	require.Eventually(t, p.Exited, 2*time.Second, 20*time.Millisecond)
}
