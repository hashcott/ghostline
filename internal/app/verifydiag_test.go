package app

import (
	"context"
	"net"
	"net/netip"
	"testing"

	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/stretchr/testify/require"
)

func verifyLeakLog(t *testing.T, h *harness) map[string]any {
	t.Helper()
	for _, e := range h.sink.events() {
		if e.Code == CodeVerifyLeak {
			return e.Params
		}
	}
	t.Fatalf("no %s log event", CodeVerifyLeak)
	return nil
}

func TestVerifyLeak_NamesUnmanagedAndDriftedAdapters(t *testing.T) {
	h := newHarness(t)
	h.res.err = &net.DNSError{Err: "no such host", Name: "x.verify.ghostline.test", IsNotFound: true}
	h.dns.report = []sysdns.AdapterDNS{
		{Adapter: sysdns.Adapter{GUID: "{A}", Alias: "Wi-Fi", IfType: 71}, IPv4: []string{"192.168.1.1"}},
		{Adapter: sysdns.Adapter{GUID: "{P}", Alias: "VNPT", IfType: 23}, IPv4: []string{"203.162.4.191"}},
		{Adapter: sysdns.Adapter{GUID: "{H}", Alias: "vEthernet", IfType: 6}},
	}
	require.Error(t, h.o.Connect(context.Background()))

	p := h.o.Snapshot().Error.Params
	require.Equal(t, verifyNXDomain, p["reason"])
	require.Equal(t, "no such host", p["detail"])
	require.Equal(t, "VNPT [ppp] v4=203.162.4.191", p["others"])
	require.Equal(t, "Wi-Fi [wifi] v4=192.168.1.1", p["drifted"])
	require.Equal(t, "Wi-Fi, VNPT", p["adapters"])
	require.Equal(t, p, verifyLeakLog(t, h))
}

func TestVerifyLeak_Reasons(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(h *harness)
		reason string
	}{
		{"timeout", func(h *harness) { h.res.err = &net.DNSError{Err: "i/o timeout", IsTimeout: true} }, verifyTimeout},
		{"other error", func(h *harness) { h.res.err = &net.DNSError{Err: "server misbehaving"} }, verifyLookupError},
		{"wrong answer", func(h *harness) { h.res.ips = []netip.Addr{netip.MustParseAddr("1.2.3.4")} }, verifyWrongAnswer},
		{"engine never saw it", func(h *harness) { h.eng.saw = false }, verifyNotSeen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.setup(h)
			require.Error(t, h.o.Connect(context.Background()))
			sn := h.o.Snapshot()
			require.Equal(t, CodeVerifyLeak, sn.Error.Code)
			require.Equal(t, c.reason, sn.Error.Params["reason"])
			require.NotContains(t, sn.Error.Params, "adapters")
		})
	}
}

// fNetshDNS is a Windows backend that can set loopback again through netsh.
type fNetshDNS struct {
	*fDNS
	onApply func()
	err     error
}

func (d *fNetshDNS) ApplyLoopbackNetsh(sysdns.Snapshot, bool) error {
	_ = d.r.add("dns.applynetsh")
	if d.err == nil && d.onApply != nil {
		d.onApply()
	}
	return d.err
}

// Windows 10 1909: SetInterfaceDnsSettings succeeded, yet the DNS client
// kept asking the old servers (NXDOMAIN, engine never saw the query). Set
// through netsh, the check passes and Ghostline connects.
func TestVerify_NetshRetryConnectsWhenEngineNeverSawQuery(t *testing.T) {
	h := newHarness(t)
	h.res.err = &net.DNSError{Err: "no such host", IsNotFound: true}
	h.eng.saw = false
	h.o.d.DNS = &fNetshDNS{fDNS: h.dns, onApply: func() { h.res.err, h.eng.saw = nil, true }}

	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
	require.Contains(t, h.r.list(), "dns.applynetsh")
}

func TestVerify_NetshRetryStillLeaking(t *testing.T) {
	h := newHarness(t)
	h.res.err = &net.DNSError{Err: "no such host", IsNotFound: true}
	h.eng.saw = false
	h.o.d.DNS = &fNetshDNS{fDNS: h.dns}

	require.Error(t, h.o.Connect(context.Background()))
	p := h.o.Snapshot().Error.Params
	require.Equal(t, CodeVerifyLeak, h.o.Snapshot().Error.Code)
	require.Equal(t, verifyNXDomain, p["reason"])
	require.Equal(t, true, p["netshRetried"])
}

// A query the engine saw needs no second try: something else is wrong.
func TestVerify_NoNetshRetryWhenEngineSawQuery(t *testing.T) {
	h := newHarness(t)
	h.res.ips = []netip.Addr{netip.MustParseAddr("1.2.3.4")}
	h.o.d.DNS = &fNetshDNS{fDNS: h.dns}

	require.Error(t, h.o.Connect(context.Background()))
	require.NotContains(t, h.r.list(), "dns.applynetsh")
	require.NotContains(t, h.o.Snapshot().Error.Params, "netshRetried")
}
