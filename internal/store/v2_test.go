package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

const settingsV1 = `{
  "version": 1, "language": "en", "mode": "advanced", "startWithWindows": true,
  "autoConnect": false, "closeToTray": true, "adapters": "auto", "testDomain": "www.google.com",
  "bootstrap": ["9.9.9.9:53"], "maxUpstreams": 3, "includeTags": ["no-filter"], "pinned": ["x"],
  "pinnedOnly": false, "probeSites": ["youtube.com"],
  "dpi": { "enabled": true, "preset": "medium", "customArgs": "", "scope": "all" },
  "fragmentDns": { "enabled": false, "chunks": 5, "delayMs": 5 },
  "updates": { "checkApp": true, "updateServerList": true },
  "advancedWindow": { "width": 1000, "height": 660 }
}`

func TestLoadSettings_V1Upgrades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(settingsV1), 0o644))
	s, recovered, err := store.LoadSettings(path)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Equal(t, 5, s.Version)
	require.Equal(t, "goodbyedpi", s.DPI.Engine)
	require.Equal(t, store.DefaultSettings().Proxy, s.Proxy)
	require.Equal(t, "zero", s.DNSBlockMode)
	require.Equal(t, "en", s.Language)
	require.Equal(t, []string{"9.9.9.9:53"}, s.Bootstrap)
	require.Equal(t, 3, s.MaxUpstreams)
	require.Equal(t, "medium", s.DPI.Preset)
}

func TestDefaultSettings_Proxy(t *testing.T) {
	p := store.DefaultSettings().Proxy
	require.Equal(t, store.ProxySettings{
		Enabled: false, Port: 8080, SystemProxy: false, ShareLAN: false,
		Fragment:  store.WebFragment{Mode: "auto", Method: "both", Chunks: 5, DelayMs: 5, AutoTimeoutMs: 3000, CacheDays: 7},
		Upstreams: []store.UpstreamProxy{},
	}, p)
	require.NoError(t, store.ValidateProxy(p))
}

func TestValidateProxy(t *testing.T) {
	mut := func(f func(p *store.ProxySettings)) store.ProxySettings {
		p := store.DefaultSettings().Proxy
		f(&p)
		return p
	}
	bad := []store.ProxySettings{
		mut(func(p *store.ProxySettings) { p.Port = 53 }),
		mut(func(p *store.ProxySettings) { p.Port = 80 }),
		mut(func(p *store.ProxySettings) { p.Port = 70000 }),
		mut(func(p *store.ProxySettings) { p.Fragment.Chunks = 1 }),
		mut(func(p *store.ProxySettings) { p.Fragment.Chunks = 65 }),
		mut(func(p *store.ProxySettings) { p.Fragment.DelayMs = 101 }),
		mut(func(p *store.ProxySettings) { p.Fragment.AutoTimeoutMs = 999 }),
		mut(func(p *store.ProxySettings) { p.Fragment.AutoTimeoutMs = 10001 }),
		mut(func(p *store.ProxySettings) { p.Fragment.CacheDays = 0 }),
		mut(func(p *store.ProxySettings) { p.Fragment.CacheDays = 91 }),
		mut(func(p *store.ProxySettings) { p.Fragment.Mode = "x" }),
		mut(func(p *store.ProxySettings) { p.Fragment.Method = "x" }),
		mut(func(p *store.ProxySettings) {
			p.Upstreams = []store.UpstreamProxy{{ID: "a", Type: "socks5", Addr: "1.2.3.4:1080"}, {ID: "a", Type: "http", Addr: "1.2.3.4:8080"}}
		}),
		mut(func(p *store.ProxySettings) {
			p.Upstreams = []store.UpstreamProxy{{ID: "Tor", Type: "socks5", Addr: "127.0.0.1:9050"}}
		}),
		mut(func(p *store.ProxySettings) {
			p.Upstreams = []store.UpstreamProxy{{ID: "t", Type: "socks4", Addr: "127.0.0.1:9050"}}
		}),
		mut(func(p *store.ProxySettings) {
			p.Upstreams = []store.UpstreamProxy{{ID: "t", Type: "socks5", Addr: "nope"}}
		}),
	}
	for i, p := range bad {
		require.Error(t, store.ValidateProxy(p), i)
	}
	ok := mut(func(p *store.ProxySettings) {
		p.Port = 1080
		p.Upstreams = []store.UpstreamProxy{{ID: "tor", Type: "socks5", Addr: "127.0.0.1:9050"}, {ID: "corp-1", Type: "http", Addr: "proxy.corp.example:3128"}}
	})
	require.NoError(t, store.ValidateProxy(ok))
}

func TestState_V1ReadsWithoutProxy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version":1,"phase":"dns_set","pid":4,"snapshot":[],"dpi":{"running":false,"pid":0}}`), 0o644))
	st, err := store.NewStateStore(path, &fakeLocker{}).Load()
	require.NoError(t, err)
	require.Equal(t, store.PhaseDNSSet, st.Phase)
	require.Nil(t, st.SysProxy)
	require.Nil(t, st.Firewall)
	require.Equal(t, 4, store.CleanState().Version)
	require.Equal(t, store.PhaseClean, store.CleanState().Phase)
}

func TestState_SysProxyRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := store.NewStateStore(path, &fakeLocker{})
	want := &store.SysProxyState{Set: true, Ours: "127.0.0.1:8080", Snapshot: &store.SysProxySnapshot{Flags: 1, Bypass: "<local>"}}
	require.NoError(t, s.Update(func(st *store.State) error {
		st.SysProxy = want
		st.AddFirewallRule("Ghostline Proxy")
		return nil
	}))
	st, err := s.Load()
	require.NoError(t, err)
	require.Equal(t, want, st.SysProxy)
	require.Equal(t, []string{"Ghostline Proxy"}, st.Firewall.Rules)
}

func TestRulesFile_RoundTripAndCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rules.json")
	f, recovered, err := store.LoadRules(path)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Empty(t, f.Rules)

	f.Rules = []rules.Rule{{Pattern: "youtube.com", Action: rules.Action{Fragment: rules.FragOn}, Enabled: true}}
	f.Lists = []lists.List{{ID: "l", Name: "L", Source: "url", URL: "https://x/y", Format: "auto", Action: "block", Enabled: true, UpdateHours: 24}}
	require.NoError(t, store.SaveRules(path, f))
	got, _, err := store.LoadRules(path)
	require.NoError(t, err)
	require.Equal(t, 1, got.Version)
	require.Equal(t, f.Rules, got.Rules)
	require.Equal(t, f.Lists, got.Lists)

	require.NoError(t, os.WriteFile(path, []byte("{bad"), 0o644))
	got, recovered, err = store.LoadRules(path)
	require.NoError(t, err)
	require.True(t, recovered)
	require.Empty(t, got.Rules)
	require.FileExists(t, path+".bak")
}

func TestFragCache_ExpiryAndPerNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frag-cache.json")
	now := time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)
	c, err := store.LoadFragCache(path, now)
	require.NoError(t, err)
	c.Add("A", "youtube.com", now.Add(7*24*time.Hour))
	c.Add("A", "old.com", now.Add(time.Hour))
	c.Add("A", "x.com", now.Add(time.Hour))
	require.True(t, c.Has("A", "youtube.com", now))
	require.False(t, c.Has("B", "youtube.com", now))
	require.False(t, c.Has("A", "old.com", now.Add(2*time.Hour)))
	require.NoError(t, c.Save(path))

	c2, err := store.LoadFragCache(path, now.Add(2*time.Hour))
	require.NoError(t, err)
	require.Equal(t, []string{"youtube.com"}, c2.List("A"))
	c2.Remove("A", "youtube.com")
	require.Empty(t, c2.List("A"))
	c2.Add("A", "a.com", now.Add(100*time.Hour))
	c2.Add("A", "b.com", now.Add(100*time.Hour))
	c2.Remove("A", "")
	require.Empty(t, c2.List("A"))
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "f.txt")
	require.NoError(t, store.WriteFileAtomic(p, []byte("hello")))
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "hello", string(b))
	entries, _ := os.ReadDir(filepath.Join(dir, "sub"))
	require.Len(t, entries, 1)
}

func TestResolvePaths_V2Files(t *testing.T) {
	p := store.ResolvePaths(filepath.Join(t.TempDir(), "ghostline.exe"), `C:\AppData`)
	require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline", "rules.json"), p.Rules)
	require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline", "frag-cache.json"), p.FragCache)
	require.Equal(t, filepath.Join(`C:\AppData`, "Ghostline", "lists"), p.ListsDir)
}
