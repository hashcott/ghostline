package procs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const tcpFixture = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0000000000000000 100 0 0 10 0
   1: 0100007F:0035 0100007F:D431 01 00000000:00000000 00:00000000 00000000     0        0 2222 1 0000000000000000 20 4 1 10 -1
   2: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 3333 1 0000000000000000 100 0 0 10 0
`

const udpFixture = `   sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  100: 3500007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000   991        0 4444 2 0000000000000000 0
`

func TestParseProcNet(t *testing.T) {
	require.Equal(t, []uint64{1111}, parseProcNet([]byte(tcpFixture), 53, true), "TCP counts only LISTEN")
	require.Equal(t, []uint64{4444}, parseProcNet([]byte(udpFixture), 53, false))
	require.Empty(t, parseProcNet([]byte(tcpFixture), 443, true))
}
