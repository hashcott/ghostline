package core

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoopbackUDP_DatagramArrives(t *testing.T) {
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	port := l.LocalAddr().(*net.UDPAddr).Port
	require.NoError(t, l.Close())
	require.NoError(t, system{}.LoopbackUDP(uint16(port)))
}

func TestLoopbackUDP_PortTaken(t *testing.T) {
	l, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer l.Close()
	require.Error(t, system{}.LoopbackUDP(uint16(l.LocalAddr().(*net.UDPAddr).Port)))
}
