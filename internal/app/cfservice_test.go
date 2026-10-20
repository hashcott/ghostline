package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/hashcott/ghostline/internal/rules"
	"github.com/stretchr/testify/require"
)

// cfEdge is a fake Cloudflare edge for speed.cloudflare.com; withDown adds
// the /__down speed endpoint.
func cfEdge(t *testing.T, withDown bool) (addr string, roots *x509.CertPool) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	caDER, _ := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	caCert, _ := x509.ParseCertificate(caDER)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), DNSNames: []string{"speed.cloudflare.com"},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &key.PublicKey, caKey)
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/cdn-cgi/trace":
			_, _ = w.Write([]byte("ip=1.2.3.4\ncolo=SIN\n"))
		case r.URL.Path == "/__down" && withDown:
			_, _ = w.Write(make([]byte, 200_000))
		default:
			http.NotFound(w, r)
		}
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	return srv.Listener.Addr().String(), pool
}

func newCF(t *testing.T, withDown bool) *toolsHarness {
	h := newTools(t)
	addr, roots := cfEdge(t, withDown)
	h.svc.cfRoots = roots
	h.svc.x.DialDirect = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	h.svc.x.NetKey = func() string { return "net1" }
	st := h.box.Get()
	st.Tools.CFScan.MaxIPs, st.Tools.CFScan.Want, st.Tools.CFScan.Concurrency = 200, 12, 16
	require.NoError(t, h.box.Save(st))
	return h
}

func waitCF(t *testing.T, h *toolsHarness) CFProgress {
	t.Helper()
	require.Eventually(t, func() bool {
		h.svc.mu.Lock()
		defer h.svc.mu.Unlock()
		return h.svc.cfCancel == nil
	}, 20*time.Second, 10*time.Millisecond)
	h.em.mu.Lock()
	defer h.em.mu.Unlock()
	evs := h.em.events[EventToolsCFScan]
	return evs[len(evs)-1].(CFProgress)
}

func TestCFScan_SavesCache(t *testing.T) {
	h := newCF(t, true)
	require.NoError(t, h.svc.StartCFScan())
	last := waitCF(t, h)
	require.False(t, last.Running)
	require.Empty(t, last.Error)
	v := h.svc.GetCFView()
	require.False(t, v.Running)
	require.Equal(t, "speed.cloudflare.com", v.Host)
	require.Len(t, v.Results, 12)
	require.Greater(t, v.Results[0].Mbps, 0.0, "the best IPs get a speed")
	c := cfscan.LoadCache(h.paths.CFScanCache)
	e, ok := c.Get("net1")
	require.True(t, ok)
	require.Len(t, e.Results, 12)
}

func TestCFScan_Busy(t *testing.T) {
	h := newCF(t, true)
	block := make(chan struct{})
	h.svc.x.DialDirect = func(ctx context.Context, _, _ string) (net.Conn, error) {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return nil, ctx.Err()
	}
	require.NoError(t, h.svc.StartCFScan())
	err := h.svc.StartCFScan()
	var ae *AppError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, CodeToolBusy, ae.Code)
	require.Equal(t, "cfscan", ae.Params["tool"])
	require.True(t, h.svc.GetCFView().Running)
	h.svc.CancelCFScan()
	close(block)
	waitCF(t, h)
}

func TestCFScan_HostInvalid(t *testing.T) {
	h := newCF(t, true)
	st := h.box.Get()
	st.Tools.CFScan.Host = "1.1.1.1"
	require.NoError(t, h.box.Save(st)) // a hand-edited settings file
	var ae *AppError
	require.ErrorAs(t, h.svc.StartCFScan(), &ae)
	require.Equal(t, CodeCFScanHostInvalid, ae.Code)
}

func TestCFScan_NoNetworkEvent(t *testing.T) {
	h := newCF(t, true)
	h.svc.x.DialDirect = func(context.Context, string, string) (net.Conn, error) {
		return nil, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}
	}
	require.NoError(t, h.svc.StartCFScan())
	last := waitCF(t, h)
	require.Equal(t, CodeCFScanNoNetwork, last.Error)
}

func TestCFScan_NoSpeedEndpointNote(t *testing.T) {
	h := newCF(t, false)
	require.NoError(t, h.svc.StartCFScan())
	last := waitCF(t, h)
	require.Equal(t, "no_speed_endpoint", last.Note)
	require.Len(t, h.svc.GetCFView().Results, 12)
}

func TestCreateCFRules(t *testing.T) {
	h := newCF(t, true)
	errs := h.svc.CreateCFRules([]string{"example.com", "*.example.com"}, []string{"104.16.1.1", "104.16.2.2"})
	require.Empty(t, errs)
	rs := h.svc.GetRules().Rules
	require.Len(t, rs, 2)
	require.Equal(t, "*.example.com", rs[1].Pattern)
	require.Equal(t, "104.16.2.2", rs[1].IPs[1].String())
	require.True(t, rs[0].Enabled)

	require.NotEmpty(t, h.svc.CreateCFRules([]string{"a.com"}, []string{"104.16.1.1", "104.16.1.2", "104.16.1.3", "104.16.1.4", "104.16.1.5"}))
	require.NotEmpty(t, h.svc.CreateCFRules([]string{"a.com"}, []string{"8.8.8.8"}), "not a Cloudflare IP")
	require.NotEmpty(t, h.svc.CreateCFRules([]string{"a.com"}, nil))
	errs = h.svc.CreateCFRules([]string{"ok.com", "~word", "10.0.0.0/8", "bad..com"}, []string{"104.16.1.1"})
	require.Len(t, errs, 3)
	require.Len(t, h.svc.GetRules().Rules, 2, "nothing partial is written")
	require.Empty(t, h.svc.CreateCFRules([]string{"example.com"}, []string{"104.16.1.1", "104.16.2.2"}))
	require.Len(t, h.svc.GetRules().Rules, 2, "duplicates are skipped")
}

func TestCFSuggestDomains(t *testing.T) {
	h := newCF(t, true)
	require.Empty(t, h.svc.SaveRulesTable([]rules.Rule{
		{Pattern: "a.com", Action: rules.Action{Block: true}, Enabled: true},
		{Pattern: "~word", Action: rules.Action{Block: true}, Enabled: true},
		{Pattern: "*.b.com", Action: rules.Action{Fragment: rules.FragOn}, Enabled: true},
		{Pattern: "a.com", Action: rules.Action{Fragment: rules.FragOn}, Enabled: true},
	}))
	require.Equal(t, []string{"a.com", "*.b.com"}, h.svc.CFSuggestDomains())
}

func TestRecheckCF_RejectsOutOfRange(t *testing.T) {
	h := newCF(t, true)
	_, err := h.svc.RecheckCF([]string{"8.8.8.8"})
	require.Error(t, err)
	rs, err := h.svc.RecheckCF([]string{"104.16.1.1"})
	require.NoError(t, err)
	require.True(t, rs[0].OK)
	e, ok := cfscan.LoadCache(h.paths.CFScanCache).Get("net1")
	require.True(t, ok)
	require.Equal(t, "104.16.1.1", e.Results[0].IP)
}

func TestCreateCFRules_UpdatesExistingRuleInsteadOfShadowing(t *testing.T) {
	h := newCF(t, true)
	require.Empty(t, h.svc.SaveRulesTable([]rules.Rule{
		{Pattern: "a.com", Action: rules.Action{Fragment: rules.FragOn}, Enabled: true},
		{Pattern: "b.com", Action: rules.Action{IPs: []netip.Addr{netip.MustParseAddr("104.16.9.9")}}, Enabled: false},
		{Pattern: "c.com", Action: rules.Action{Block: true}, Enabled: true},
	}))
	require.Equal(t, []string{"a.com", "b.com", "c.com"}, h.svc.CFSuggestDomains())

	require.Empty(t, h.svc.CreateCFRules([]string{"a.com", "b.com"}, []string{"104.16.1.1"}))
	rs := h.svc.GetRules().Rules
	require.Len(t, rs, 3, "no shadowed duplicate is appended")
	require.Equal(t, rules.FragOn, rs[0].Fragment, "other actions are kept")
	require.Equal(t, "104.16.1.1", rs[0].IPs[0].String())
	require.Len(t, rs[1].IPs, 1)
	require.Equal(t, "104.16.1.1", rs[1].IPs[0].String(), "stale clean IPs are replaced")
	require.True(t, rs[1].Enabled)

	errs := h.svc.CreateCFRules([]string{"c.com"}, []string{"104.16.1.1"})
	require.Len(t, errs, 1, "a block rule for the same pattern would win")
	require.Equal(t, 1, errs[0].Line)
	require.True(t, h.svc.GetRules().Rules[2].Block)
}

func blockingDial(block chan struct{}) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		select {
		case <-block:
			return nil, net.ErrClosed
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func TestRecheckCF_BusyCapAndCancel(t *testing.T) {
	h := newCF(t, true)
	block := make(chan struct{})
	defer close(block)
	h.svc.x.DialDirect = blockingDial(block)
	require.NoError(t, h.svc.StartCFScan())
	_, err := h.svc.RecheckCF([]string{"104.16.1.1"})
	require.Equal(t, CodeToolBusy, code(t, err), "no recheck while a scan runs")
	h.svc.CancelCFScan()
	waitCF(t, h)

	many := make([]string, 101)
	for i := range many {
		many[i] = fmt.Sprintf("104.16.%d.%d", i/200, 1+i%200)
	}
	_, err = h.svc.RecheckCF(many)
	require.Error(t, err)

	// A recheck can be cancelled like a scan.
	done := make(chan error, 1)
	go func() { _, err := h.svc.RecheckCF([]string{"104.16.1.1", "104.16.1.2"}); done <- err }()
	require.Eventually(t, func() bool { return h.svc.GetCFView().Running }, 2*time.Second, 5*time.Millisecond)
	h.svc.CancelCFScan()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("recheck did not stop on cancel")
	}
}

func TestCFScan_CancelKeepsCachedResults(t *testing.T) {
	h := newCF(t, true)
	old := cfscan.Result{IP: "104.16.50.50", OK: true, LatencyMs: 30, Colo: "SIN"}
	c := cfscan.LoadCache(h.paths.CFScanCache)
	c.Put("net1", time.Now().Add(-time.Hour), "speed.cloudflare.com", []cfscan.Result{old})
	require.NoError(t, cfscan.SaveCache(h.paths.CFScanCache, c))

	block := make(chan struct{})
	defer close(block)
	h.svc.x.DialDirect = blockingDial(block)
	require.NoError(t, h.svc.StartCFScan())
	h.svc.CancelCFScan()
	waitCF(t, h)
	e, ok := cfscan.LoadCache(h.paths.CFScanCache).Get("net1")
	require.True(t, ok)
	require.NotEmpty(t, e.Results)
	require.Equal(t, "104.16.50.50", e.Results[0].IP, "a cancelled scan does not throw away the last full scan")
}
