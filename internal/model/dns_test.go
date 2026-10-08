package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDNSSnapshot_LabelServersEmpty(t *testing.T) {
	var zero DNSSnapshot
	require.True(t, zero.Empty())
	require.Empty(t, zero.Servers())

	win := DNSSnapshot{Backend: "windows", Windows: []AdapterSnapshot{
		{GUID: "{A}", Alias: "Wi-Fi", IPv4: FamilyDNS{Mode: DNSModeStatic, Servers: []string{"1.1.1.1"}}, IPv6: FamilyDNS{Mode: DNSModeStatic, Servers: []string{"2606:4700::1111"}}},
		{GUID: "{B}", Alias: "Ethernet", IPv4: FamilyDNS{Mode: DNSModeDHCP}},
	}}
	require.False(t, win.Empty())
	require.Equal(t, "Wi-Fi", win.Label())
	require.Equal(t, []string{"1.1.1.1", "2606:4700::1111"}, win.Servers())

	lin := DNSSnapshot{Backend: "networkmanager", Linux: &LinuxDNS{NMGlobal: &NMGlobalDNS{}, Servers: []string{"192.168.1.1"}}}
	require.False(t, lin.Empty())
	require.Equal(t, "networkmanager", lin.Label())
	require.Equal(t, []string{"192.168.1.1"}, lin.Servers())
}
