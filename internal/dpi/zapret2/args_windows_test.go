package zapret2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// winws2's capture flags must stay exactly what v0.5 passed.
func TestArgs_WindowsUnchanged(t *testing.T) {
	require.Equal(t, []string{"--wf-tcp-out=80,443", "--wf-dup-check=1"}, interceptArgs(false))
	require.Equal(t, []string{"--wf-tcp-out=80,443", "--wf-udp-out=443", "--wf-dup-check=1"}, interceptArgs(true))
	require.Equal(t, "winws2.exe", exeName)
	require.Contains(t, Pinned, "winws2.exe")
	require.Contains(t, Pinned, "WinDivert64.sys")
	require.NotContains(t, Pinned, "nfqws2")
}
