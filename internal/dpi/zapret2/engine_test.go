package zapret2

import (
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/hashcott/ghostline/internal/dpi/zapret2/strategies"
	"github.com/stretchr/testify/require"
)

var testList = strategies.List{Version: 1, Zapret2: []strategies.Strategy{
	{ID: "z-split", Name: map[string]string{"vi": "Nhẹ", "en": "Light"}, TCP: []string{"--lua-desync=multisplit:pos=1,midsld"}},
	{ID: "z-fake", Name: map[string]string{"vi": "Mạnh", "en": "Strong"},
		TCP:  []string{"--lua-desync=fake:blob=fake_default_tls:badsum", "--lua-desync=multidisorder:pos=1,midsld"},
		QUIC: []string{"--lua-desync=fake:blob=fake_default_quic:repeats=6"}},
}}

func newTest() dpi.Engine { return New(func() strategies.List { return testList }) }

// luaInits load the strategies' Lua libraries; the capture flags before
// them are per OS (args_<os>_test.go pins them).
var luaInits = []string{
	"--lua-init=@lua/zapret-lib.lua",
	"--lua-init=@lua/zapret-antidpi.lua",
	"--lua-init=@lua/zapret-auto.lua",
}

var head = cat(interceptArgs(false), luaInits)

func cat(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestArgs_AllScopeNoQUIC(t *testing.T) {
	a, err := newTest().Args(dpi.Plan{Strategy: "z-split", Scope: dpi.ScopeAll})
	require.NoError(t, err)
	require.Equal(t, cat(head, []string{"--filter-tcp=80,443", "--lua-desync=multisplit:pos=1,midsld"}), a)
}

func TestArgs_BlacklistWithQUIC(t *testing.T) {
	a, err := newTest().Args(dpi.Plan{Strategy: "z-fake", Scope: dpi.ScopeBlacklist, Blacklist: "blacklist.txt"})
	require.NoError(t, err)
	require.Equal(t, cat(
		interceptArgs(true), luaInits,
		[]string{"--filter-tcp=80,443", "--hostlist=blacklist.txt",
			"--lua-desync=fake:blob=fake_default_tls:badsum", "--lua-desync=multidisorder:pos=1,midsld"},
		[]string{"--new", "--filter-udp=443", "--filter-l7=quic", "--hostlist=blacklist.txt",
			"--lua-desync=fake:blob=fake_default_quic:repeats=6"},
	), a)
}

func TestArgs_BlacklistWithAutoHostlist(t *testing.T) {
	a, err := newTest().Args(dpi.Plan{Strategy: "z-fake", Scope: dpi.ScopeBlacklist, Blacklist: "blacklist.txt", AutoHostlist: "autohostlist.txt"})
	require.NoError(t, err)
	n := 0
	for i, x := range a {
		if x == "--hostlist-auto=autohostlist.txt" {
			n++
			require.Equal(t, "--hostlist=blacklist.txt", a[i-1])
		}
	}
	require.Equal(t, 2, n, "one per profile")
}

func TestArgs_AllScopeIgnoresAutoHostlist(t *testing.T) {
	a, err := newTest().Args(dpi.Plan{Strategy: "z-fake", Scope: dpi.ScopeAll, AutoHostlist: "autohostlist.txt"})
	require.NoError(t, err)
	for _, x := range a {
		require.NotContains(t, x, "--hostlist")
	}
}

func TestArgs_Custom(t *testing.T) {
	a, err := newTest().Args(dpi.Plan{Strategy: "custom", Custom: "--lua-desync=fake:blob=fake_default_tls --lua-desync=multisplit:pos=2"})
	require.NoError(t, err)
	require.Equal(t, cat(head, []string{"--filter-tcp=80,443", "--lua-desync=fake:blob=fake_default_tls", "--lua-desync=multisplit:pos=2"}), a)
	_, err = newTest().Args(dpi.Plan{Strategy: "custom", Custom: "--lua-init=@x.lua"})
	require.ErrorIs(t, err, dpi.ErrForbiddenFlag)
	_, err = newTest().Args(dpi.Plan{Strategy: "custom", Custom: ""})
	require.Error(t, err, "a profile with no desync does nothing")
}

func TestArgs_UnknownStrategy(t *testing.T) {
	_, err := newTest().Args(dpi.Plan{Strategy: "gone"})
	require.Error(t, err)
}

func TestStrategies_FollowList(t *testing.T) {
	l := testList
	e := New(func() strategies.List { return l })
	require.Equal(t, []dpi.Strategy{{ID: "z-split", Name: testList.Zapret2[0].Name}, {ID: "z-fake", Name: testList.Zapret2[1].Name}}, e.Strategies())
	l = strategies.List{Version: 2, Zapret2: testList.Zapret2[1:]}
	require.Len(t, e.Strategies(), 1)
	require.Equal(t, "z-fake", e.Strategies()[0].ID)
}

func TestEngineIdentity(t *testing.T) {
	e := newTest()
	require.Equal(t, "zapret2", e.ID())
	require.Equal(t, exeName, e.Exe())
	require.True(t, e.HotReloadsLists())
	require.Equal(t, Pinned, e.Files())
	require.NoError(t, e.ValidateCustom("--lua-desync=pass"))
	require.Error(t, e.ValidateCustom("--lua-desync=luaexec"))
}
