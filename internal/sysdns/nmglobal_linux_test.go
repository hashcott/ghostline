package sysdns

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/hashcott/ghostline/internal/model"
	"github.com/stretchr/testify/require"
)

func TestNMGlobal_RoundTrip(t *testing.T) {
	g := model.NMGlobalDNS{
		Searches: []string{"lan"},
		Domains:  map[string]model.NMDomain{"*": {Servers: []string{"127.0.0.1", "::1"}}},
	}
	v := globalToVariant(g)
	dom, ok := v["domains"].Value().(map[string]dbus.Variant)
	require.True(t, ok, "domains is a{sv}")
	star, ok := dom["*"].Value().(map[string]dbus.Variant)
	require.True(t, ok)
	require.Equal(t, []string{"127.0.0.1", "::1"}, star["servers"].Value())
	require.Equal(t, g, variantToGlobal(v))
}

func TestNMGlobal_EmptyIsEmptyMap(t *testing.T) {
	require.Empty(t, globalToVariant(model.NMGlobalDNS{}))
	require.Equal(t, model.NMGlobalDNS{}, variantToGlobal(map[string]dbus.Variant{}))
}
