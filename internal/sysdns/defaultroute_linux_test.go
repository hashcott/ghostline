package sysdns

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"
)

// resolved refuses runtime settings on links systemd-networkd manages
// ("Link eth0 is managed."): networkd sets them instead. Without that,
// the link keeps DefaultRoute=yes and its DNS servers still get queries.
func TestSetLinkDefaultRoute_NetworkdManagedLink(t *testing.T) {
	var got []string
	resolved := func(int32, bool) error {
		got = append(got, "resolved")
		return dbus.Error{Name: "org.freedesktop.resolve1.LinkBusy", Body: []any{"Link eth0 is managed."}}
	}
	networkd := func(ifindex int32, on bool) error {
		got = append(got, "networkd")
		require.Equal(t, int32(2), ifindex)
		require.False(t, on)
		return nil
	}
	require.NoError(t, setLinkDefaultRoute(resolved, networkd, 2, false))
	require.Equal(t, []string{"resolved", "networkd"}, got)
}

func TestSetLinkDefaultRoute_ResolvedOwnsTheLink(t *testing.T) {
	called := false
	require.NoError(t, setLinkDefaultRoute(func(int32, bool) error { return nil }, func(int32, bool) error { called = true; return nil }, 2, false))
	require.False(t, called)
}

func TestSetLinkDefaultRoute_GoneLink(t *testing.T) {
	gone := func(int32, bool) error { return dbus.Error{Name: "org.freedesktop.resolve1.NoSuchLink"} }
	require.ErrorIs(t, setLinkDefaultRoute(gone, nil, 9, true), errNoSuchLink)
	boom := errors.New("networkd down")
	busy := func(int32, bool) error { return dbus.Error{Name: "org.freedesktop.resolve1.LinkBusy"} }
	require.ErrorIs(t, setLinkDefaultRoute(busy, func(int32, bool) error { return boom }, 2, false), boom)
}
