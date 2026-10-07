package app

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/hashcott/ghostline/internal/rules/lists"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

type fSetupPage struct {
	mu      sync.Mutex
	files   dnsserver.SetupFiles
	onStop  func()
	addrs   []netip.AddrPort
	life    time.Duration
	running bool
}

func (p *fSetupPage) Start(addrs []netip.AddrPort, life time.Duration) error {
	p.mu.Lock()
	p.addrs, p.life, p.running = addrs, life, true
	p.mu.Unlock()
	return nil
}
func (p *fSetupPage) Stop() error {
	p.mu.Lock()
	was := p.running
	p.running = false
	p.mu.Unlock()
	if was && p.onStop != nil {
		p.onStop()
	}
	return nil
}
func (p *fSetupPage) Running() bool            { p.mu.Lock(); defer p.mu.Unlock(); return p.running }
func (p *fSetupPage) Remaining() time.Duration { return time.Minute }

type svc2B struct {
	*svcHarness
	srv   *fDNSServer
	certs *fCerts
	page  *fSetupPage
	saved map[string][]byte
	ssid  string
}

func newSvc2B(t *testing.T) *svc2B {
	sh := newSvc(t)
	h := &svc2B{svcHarness: sh, srv: &fDNSServer{r: sh.r}, saved: map[string][]byte{}}
	h.certs = &fCerts{r: sh.r, t: t, states: sh.states, store: certstore.NewFake()}
	d := &h.o.d
	d.Proxy = &fProxy{r: sh.r}
	d.Firewall = &fFirewall{r: sh.r, states: sh.states, t: t}
	d.DNSServer, d.Certs = h.srv, h.certs
	d.LANAddrs = func() []netip.Addr { return []netip.Addr{netip.MustParseAddr("192.168.1.5")} }
	d.SetMITM = func(l mitm.LeafSource) {
		if l == nil {
			_ = sh.r.add("mitm.clear")
		}
	}
	d.MITMSelfTest = func(context.Context) error { return nil }
	d.Rules = func() *rules.Compiled { return compileRules(t, "youtube.com sni=www.google.com") }
	h.svc.x.NewSetupPage = func(f dnsserver.SetupFiles, onStop func()) SetupPage {
		h.page = &fSetupPage{files: f, onStop: onStop}
		return h.page
	}
	h.svc.x.SaveFile = func(name string, b []byte) error { h.saved[name] = b; return nil }
	h.svc.x.CurrentSSID = func() string { return h.ssid }
	st := sh.box.Get()
	st.Proxy.Enabled = true
	st.DNSServer = store.DNSServerSettings{Enabled: true, ShareLAN: true, DoHPort: 443}
	require.NoError(t, sh.box.Save(st))
	return h
}

func TestSetDNSServer_Validation(t *testing.T) {
	h := newSvc2B(t)
	require.Error(t, h.svc.SetDNSServer(true, true, 8053))
	require.Error(t, h.svc.SetDNSServer(true, true, 53))
	require.NoError(t, h.svc.SetDNSServer(true, false, 8443))
	got := h.box.Get().DNSServer
	require.True(t, got.Enabled)
	require.False(t, got.ShareLAN)
	require.Equal(t, 8443, got.DoHPort)
}

func TestFakeSNI_AckThenEnable(t *testing.T) {
	h := newSvc2B(t)
	err := h.svc.SetFakeSNI(true)
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeFakeSNINotAcked, ae.Code)
	require.NoError(t, h.svc.AckFakeSNIWarning())
	require.Equal(t, FakeSNIWarningVersion, h.box.Get().FakeSNI.AckVersion)
	require.NoError(t, h.svc.SetFakeSNI(true))
	require.True(t, h.box.Get().FakeSNI.Enabled)
	require.True(t, h.svc.GetFakeSNIView().Ack)
}

func TestOpenSetupPage_WritesStateFirstAndCleansUp(t *testing.T) {
	h := newSvc2B(t)
	h.setSSID(t, "Home")
	require.NoError(t, h.svc.Connect())
	url, err := h.svc.OpenSetupPage()
	require.NoError(t, err)
	require.Equal(t, "http://192.168.1.5:8053/", url)
	require.Contains(t, h.r.list(), "firewall.add:"+firewall.RuleSetup) // fake asserts state first
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("192.168.1.5:8053")}, h.page.addrs)
	require.Equal(t, 10*time.Minute, h.page.life)
	require.Equal(t, "Home", h.page.files.SSID)
	mc, err := h.page.files.MobileConfig("Home")
	require.NoError(t, err)
	require.Contains(t, string(mc), "<string>Home</string>")
	require.NotEmpty(t, h.page.files.Fingerprint)
	info := h.svc.GetDeviceInfo()
	require.Equal(t, url, info.SetupURL)
	require.Equal(t, []string{"192.168.1.5"}, info.DNSAddrs)
	require.Equal(t, []string{"https://192.168.1.5/dns-query"}, info.DoHURLs)

	require.NoError(t, h.svc.CloseSetupPage())
	require.Contains(t, h.r.list(), "firewall.delete:"+firewall.RuleSetup)
	st, _ := h.states.Load()
	require.NotContains(t, st.Firewall.Rules, firewall.RuleSetup)
}

func TestOpenSetupPage_NoSSIDNoProfile(t *testing.T) {
	h := newSvc2B(t)
	require.NoError(t, h.svc.Connect())
	_, err := h.svc.OpenSetupPage()
	require.NoError(t, err)
	// No saved name: the phone enters it; the profile is built for that name.
	require.Empty(t, h.page.files.SSID)
	mc, err := h.page.files.MobileConfig("Phone Wi-Fi")
	require.NoError(t, err)
	require.Contains(t, string(mc), "<string>Phone Wi-Fi</string>")
}

func TestOpenSetupPage_NeedsSharedDNSServer(t *testing.T) {
	h := newSvc2B(t)
	_, err := h.svc.OpenSetupPage()
	require.Error(t, err)
}

func TestDisconnect_ClosesSetupPage(t *testing.T) {
	h := newSvc2B(t)
	require.NoError(t, h.svc.Connect())
	_, err := h.svc.OpenSetupPage()
	require.NoError(t, err)
	require.NoError(t, h.svc.Disconnect())
	require.False(t, h.page.Running())
	require.Contains(t, h.r.list(), "firewall.delete:"+firewall.RuleSetup)
}

func TestGetDeviceInfo_SuggestsCurrentSSID(t *testing.T) {
	h := newSvc2B(t)
	h.ssid = "Cafe"
	require.Equal(t, "Cafe", h.svc.GetDeviceInfo().SSID)
	h.setSSID(t, "Home")
	require.Equal(t, "Home", h.svc.GetDeviceInfo().SSID)
}

func TestSaveDeviceFiles(t *testing.T) {
	h := newSvc2B(t)
	h.setSSID(t, "Home")
	require.NoError(t, h.svc.Connect())
	require.NoError(t, h.svc.SaveDeviceFiles())
	require.NotEmpty(t, h.saved["ghostline-lan-ca.crt"])
	require.NotEmpty(t, h.saved["ghostline.mobileconfig"])
}

func TestRemoveAllCerts_StopsPhasesFirst(t *testing.T) {
	h := newSvc2B(t)
	require.NoError(t, h.svc.AckFakeSNIWarning())
	require.NoError(t, h.svc.SetFakeSNI(true))
	require.NoError(t, h.svc.Connect())
	require.True(t, h.svc.GetSnapshot().FakeSNI.Active)
	require.NoError(t, h.svc.RemoveAllCerts())
	requireOrder(t, h.r.list(), "mitm.clear", "certs.remove", "dns.stopserve", "certs.removelan")
	st := h.box.Get()
	require.False(t, st.DNSServer.Enabled)
	require.False(t, st.FakeSNI.Enabled)
	left, _ := h.svc.ListCerts()
	require.Empty(t, left)
}

func TestSetListTrustedForSNI(t *testing.T) {
	rh := newRulesSvc(t)
	rh.svc.rmu.Lock()
	rh.svc.rf.Lists = append(rh.svc.rf.Lists, lists.List{ID: "p", Name: "P", Source: "url", URL: "https://example.com/p.txt", Format: "auto", Action: "perLine"})
	rh.svc.rmu.Unlock()
	require.NoError(t, rh.svc.SetListTrustedForSNI("p", true))
	require.True(t, rh.svc.GetRules().Lists[0].TrustedForSNI)
	require.Error(t, rh.svc.SetListTrustedForSNI("nope", true))
}

func (h *svc2B) setSSID(t *testing.T, ssid string) {
	require.NoError(t, h.svc.SetIOSSSID(ssid))
}

func TestAddList_FakeSNIPresetKeepsFlags(t *testing.T) {
	rh := newRulesSvc(t)
	var preset lists.CatalogItem
	for _, it := range rh.svc.Catalog() {
		if it.Category == "fakesni" {
			preset = it
		}
	}
	l, err := rh.svc.AddList(lists.List{Name: preset.Name, Source: "url", URL: preset.URL, Format: preset.Format,
		Action: preset.Action, Signed: preset.Signed, TrustedForSNI: preset.TrustedForSNI})
	require.NoError(t, err)
	require.True(t, l.Signed)
	require.True(t, l.TrustedForSNI)
	_, err = rh.svc.AddList(lists.List{Name: "x", Source: "url", URL: "https://example.com/x.txt", Format: "domains", Action: "perLine"})
	require.Error(t, err, "per-line actions need the ghostline format")
}

// The UI saves its whole settings copy; a stale copy must not undo what
// SetFakeSNI or SetDNSServer changed (they own those blocks).
func TestSaveSettings_StaleCopyKeepsFakeSNIAndDNSServer(t *testing.T) {
	h := newSvc2B(t)
	require.NoError(t, h.svc.AckFakeSNIWarning())
	require.NoError(t, h.svc.SetFakeSNI(true))
	stale := h.box.Get() // the UI's copy: Fake SNI on, DNS server on
	require.NoError(t, h.svc.SetFakeSNI(false))
	require.NoError(t, h.svc.SetDNSServer(false, false, 8443))

	stale.Language = "en" // an unrelated change from another page
	require.NoError(t, h.svc.SaveSettings(stale))
	got := h.box.Get()
	require.Equal(t, "en", got.Language)
	require.False(t, got.FakeSNI.Enabled, "a stale UI copy turned Fake SNI back on")
	require.False(t, got.DNSServer.Enabled)
	require.Equal(t, 8443, got.DNSServer.DoHPort)
}

func TestGetDeviceInfo_WifiSuggestions(t *testing.T) {
	h := newSvc2B(t)
	h.ssid = "Cafe"
	h.svc.x.WifiNames = func() []string { return []string{"Home", "Cafe", "Office"} }
	info := h.svc.GetDeviceInfo()
	require.Equal(t, []string{"Cafe", "Home", "Office"}, info.WifiSuggestions, "current Wi-Fi first, no duplicates")
}
