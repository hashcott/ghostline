package zapret2

import (
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/stretchr/testify/require"
)

func TestArgs_LinuxQueueAndUser(t *testing.T) {
	require.Equal(t, []string{"--qnum=200", "--fwmark=0x40000000", "--user=nobody"}, interceptArgs(true))
	a, err := newTest().Args(dpi.Plan{Strategy: "z-fake", Scope: dpi.ScopeAll})
	require.NoError(t, err)
	for _, x := range a {
		require.False(t, strings.HasPrefix(x, "--wf-"), x)
	}
	require.Subset(t, a, luaInits)
	require.Equal(t, "nfqws2", exeName)
}

func TestPins_LinuxBinary(t *testing.T) {
	require.Equal(t, "de1414b1e0f9a5659d438cddcb0c7e9099b4533bf5c8a3cf841c7bc7e5aa1473", Pinned["nfqws2"])
	require.NotContains(t, Pinned, "winws2.exe")
}
