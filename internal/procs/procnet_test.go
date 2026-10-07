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
	require.Empty(t, parseProcNet([]byte(udpFixture), 53, false), "resolved's stub on 127.0.0.53 does not conflict")
	require.Empty(t, parseProcNet([]byte(tcpFixture), 443, true))
}

// Review I3: sockets on another loopback address (resolved's stub on
// 127.0.0.53, dnsmasq on 127.0.1.1) never conflict with Ghostline's
// 127.0.0.1 / ::1; the wildcard and LAN addresses can.
const udp4Mixed = `   sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  1: 3500007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000   991        0 1 2 0000000000000000 0
  2: 0101007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000   991        0 2 2 0000000000000000 0
  3: 0100007F:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 3 2 0000000000000000 0
  4: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 4 2 0000000000000000 0
  5: 0501A8C0:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 5 2 0000000000000000 0
`

const udp6Mixed = `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  1: 00000000000000000000000001000000:0035 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 11 2 0000000000000000 0
  2: 00000000000000000000000000000000:0035 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 12 2 0000000000000000 0
  3: 0000000000000000FFFF00003500007F:0035 00000000000000000000000000000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 13 2 0000000000000000 0
`

func TestParseProcNet_OnlyConflictingAddresses(t *testing.T) {
	require.Equal(t, []uint64{3, 4, 5}, parseProcNet([]byte(udp4Mixed), 53, false))
	require.Equal(t, []uint64{11, 12}, parseProcNet([]byte(udp6Mixed), 53, false))
}

// Review I3: only system services; a user unit named like one must not
// make root stop the system unit of that name.
func TestUnitFromCgroup(t *testing.T) {
	require.Equal(t, "dnsmasq.service", unitFromCgroup("0::/system.slice/dnsmasq.service\n"))
	require.Equal(t, "", unitFromCgroup("0::/user.slice/user-1000.slice/user@1000.service/app.slice/firewalld.service\n"))
	require.Equal(t, "", unitFromCgroup("0::/user.slice/user-1000.slice/session-2.scope\n"))
}

// Review I3: never stop what DNS itself or Ghostline runs on.
func TestStoppable(t *testing.T) {
	for _, u := range []string{"systemd-resolved.service", "NetworkManager.service", "ghostline.service"} {
		require.Error(t, stoppable(u), u)
	}
	require.NoError(t, stoppable("dnsmasq.service"))
	require.Error(t, stoppable("../x.service"))
	require.Error(t, stoppable("dnsmasq"))
}
