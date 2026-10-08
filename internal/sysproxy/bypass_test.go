package sysproxy_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

// WinINET has no CIDR, so its list spells out 172.16-31; it must stay what
// v0.5 wrote.
func TestBypass_WinINETUnchanged(t *testing.T) {
	require.Equal(t, "<local>;localhost;127.*;10.*;172.16.*;172.17.*;172.18.*;172.19.*;172.20.*;172.21.*;172.22.*;172.23.*;172.24.*;172.25.*;172.26.*;172.27.*;172.28.*;172.29.*;172.30.*;172.31.*;192.168.*;[::1]", sysproxy.BypassWinINET())
}

func TestBypass_GNOMEAndKDE(t *testing.T) {
	require.Equal(t, "['localhost', '127.0.0.0/8', '::1', '10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16']", sysproxy.BypassGNOME())
	require.Equal(t, "localhost,127.0.0.0/8,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16", sysproxy.BypassKDE())
}
