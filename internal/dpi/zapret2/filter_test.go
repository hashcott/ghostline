package zapret2

import (
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/stretchr/testify/require"
)

func TestFilter_PortsList(t *testing.T) {
	require.Equal(t, "80,443", portsCSV(Filter[0]))
	require.Equal(t, dpi.CaptureRule{Proto: "udp", Ports: []int{443}, OutPackets: 5, InPackets: 3, QUIC: true}, Filter[1])
	require.Equal(t, 20, Filter[0].OutPackets)
	require.Equal(t, 10, Filter[0].InPackets)
}

func TestPins_LuaInEveryBuild(t *testing.T) {
	for name := range LuaPins {
		require.Contains(t, Pinned, name)
	}
}
