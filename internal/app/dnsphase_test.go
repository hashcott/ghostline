package app

import (
	"context"
	"net/netip"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/stretchr/testify/require"
)

type fDNSServer struct {
	r    *rec
	mu   sync.Mutex
	runs []engine.ServeConfig
	skip map[netip.AddrPort]string
	err  error
}

func (s *fDNSServer) Serve(_ context.Context, sc engine.ServeConfig) (engine.ServeResult, error) {
	if err := s.r.add("dns.serve"); err != nil {
		return engine.ServeResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = append(s.runs, sc)
	res := engine.ServeResult{Skipped: map[netip.AddrPort]string{}}
	if s.err != nil {
		return res, s.err
	}
	for _, a := range append(append([]netip.AddrPort(nil), sc.Plain...), sc.DoH...) {
		if why, ok := s.skip[a]; ok {
			res.Skipped[a] = why
			continue
		}
		res.Bound = append(res.Bound, a)
	}
	return res, nil
}
func (s *fDNSServer) StopServe(context.Context) error { return s.r.add("dns.stopserve") }
func (s *fDNSServer) SelfTest(context.Context) error  { return s.r.add("dns.selftest") }
func (s *fDNSServer) last() engine.ServeConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[len(s.runs)-1]
}

// fCerts keeps installed certs in a certstore.Fake and checks the
// write-ahead rule for session CAs.
type fCerts struct {
	r      *rec
	t      *testing.T
	states *fStates
	store  *certstore.Fake
	lan    *certs.CA
	lanErr error
	// failAfterInstall adds the certificate, then reports a read-back error.
	failAfterInstall bool
}

func (c *fCerts) LANCA(context.Context) (*certs.CA, error) {
	if err := c.r.add("certs.lanca"); err != nil {
		return nil, err
	}
	if c.lanErr != nil {
		return nil, c.lanErr
	}
	if c.lan == nil {
		var err error
		if c.lan, err = certs.NewLANCA("TEST", time.Now()); err != nil {
			return nil, err
		}
	}
	return c.lan, c.store.Install(c.lan.DER)
}
func (c *fCerts) ResetLANCA(ctx context.Context) (*certs.CA, error) {
	_ = c.r.add("certs.resetlan")
	c.lan = nil
	return c.LANCA(ctx)
}
func (c *fCerts) RemoveLANCA(context.Context) error {
	if c.lan != nil {
		_ = c.store.Remove(c.lan.Thumbprint())
	}
	c.lan = nil
	return c.r.add("certs.removelan")
}
func (c *fCerts) InstallSession(der []byte) error {
	st, err := c.states.Load()
	require.NoError(c.t, err)
	require.NotNil(c.t, st.Certs, "session CA installed before its thumbprint was persisted")
	require.Contains(c.t, st.Certs.Session, certstore.Thumbprint(der))
	if err := c.r.add("certs.install"); err != nil {
		return err
	}
	if err := c.store.Install(der); err != nil {
		return err
	}
	if c.failAfterInstall {
		return certstore.ErrNotInstalled
	}
	return nil
}
func (c *fCerts) RemoveSession(thumb string) error {
	if err := c.r.add("certs.remove"); err != nil {
		return err
	}
	return c.store.Remove(thumb)
}
func (c *fCerts) List() ([]certstore.Cert, error) { return c.store.List("Ghostline") }

type dnsHarness struct {
	*proxyHarness
	srv   *fDNSServer
	certs *fCerts
	lanMu sync.Mutex
	lan   []netip.Addr
}

func newDNSHarness(t *testing.T) *dnsHarness {
	ph := newProxyHarness(t, true)
	h := &dnsHarness{proxyHarness: ph, srv: &fDNSServer{r: ph.r},
		certs: &fCerts{r: ph.r, t: t, states: ph.states, store: certstore.NewFake()},
		lan:   []netip.Addr{netip.MustParseAddr("192.168.1.5")}}
	h.o.d.DNSServer, h.o.d.Certs = h.srv, h.certs
	h.o.d.LANAddrs = func() []netip.Addr { h.lanMu.Lock(); defer h.lanMu.Unlock(); return slices.Clone(h.lan) }
	h.settings.DNSServer.Enabled = true
	h.settings.DNSServer.ShareLAN = true
	h.settings.DNSServer.DoHPort = 443
	return h
}

func (h *dnsHarness) setLAN(a ...string) {
	h.lanMu.Lock()
	defer h.lanMu.Unlock()
	h.lan = nil
	for _, s := range a {
		h.lan = append(h.lan, netip.MustParseAddr(s))
	}
}

func TestPhaseD_StepsInOrder(t *testing.T) {
	h := newDNSHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	requireOrder(t, h.r.list(), "proxy.start", "certs.lanca", "firewall.add:"+firewall.RuleDNSTCP, "firewall.add:"+firewall.RuleDNSUDP, "dns.serve", "dns.selftest")
	sc := h.srv.last()
	require.Contains(t, sc.DoH, netip.MustParseAddrPort("127.0.0.1:443"))
	require.Contains(t, sc.DoH, netip.MustParseAddrPort("192.168.1.5:443"))
	require.Equal(t, []netip.AddrPort{netip.MustParseAddrPort("192.168.1.5:53")}, sc.Plain)
	leaf := sc.Cert()
	require.NotNil(t, leaf)
	require.NoError(t, leaf.Leaf.VerifyHostname("192.168.1.5"))
	require.NoError(t, leaf.Leaf.VerifyHostname("dns.ghostline.lan"))
	sn := h.o.Snapshot()
	require.Equal(t, StatusProtected, sn.Status)
	require.True(t, sn.DNSServer.Running)
	st, _ := h.states.Load()
	require.Subset(t, st.Firewall.Rules, []string{firewall.RuleDNSTCP, firewall.RuleDNSUDP})
}

func TestPhaseD_LoopbackOnlyWithoutShare(t *testing.T) {
	h := newDNSHarness(t)
	h.settings.DNSServer.ShareLAN = false
	require.NoError(t, h.o.Connect(context.Background()))
	require.NotContains(t, h.r.list(), "firewall.add:"+firewall.RuleDNSTCP)
	require.Empty(t, h.srv.last().Plain)
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
}

func TestPhaseD_Disabled(t *testing.T) {
	h := newDNSHarness(t)
	h.settings.DNSServer.Enabled = false
	require.NoError(t, h.o.Connect(context.Background()))
	require.NotContains(t, h.r.list(), "certs.lanca")
	require.NotContains(t, h.r.list(), "dns.serve")
}

func TestPhaseD_NoLoopbackBind(t *testing.T) {
	h := newDNSHarness(t)
	h.srv.err = engine.ErrNoLoopbackDoH
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, StatusDegraded, sn.Status)
	require.Contains(t, sn.Reasons, reasonDNSServer)
	require.Equal(t, CodeDNSServerPortInUse, sn.DNSServer.Error.Code)
	require.False(t, sn.DNSServer.Running)
	require.Contains(t, h.r.list(), "firewall.delete:"+firewall.RuleDNSTCP)
	require.NotContains(t, h.r.list(), "dns.restore", "DNS must stay protected")
	st, _ := h.states.Load()
	if st.Firewall != nil {
		require.NotContains(t, st.Firewall.Rules, firewall.RuleDNSTCP)
	}
}

func TestPhaseD_PartialBind(t *testing.T) {
	h := newDNSHarness(t)
	h.setLAN("192.168.1.5", "10.0.0.7")
	h.srv.skip = map[netip.AddrPort]string{netip.MustParseAddrPort("10.0.0.7:53"): "in use", netip.MustParseAddrPort("10.0.0.7:443"): "in use"}
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.True(t, sn.DNSServer.Running)
	require.Equal(t, "in use", sn.DNSServer.Skipped["10.0.0.7:53"])
	require.Equal(t, StatusProtected, sn.Status)
}

func TestPhaseD_ShareButNoLANBound(t *testing.T) {
	h := newDNSHarness(t)
	h.srv.skip = map[netip.AddrPort]string{netip.MustParseAddrPort("192.168.1.5:53"): "ICS", netip.MustParseAddrPort("192.168.1.5:443"): "ICS"}
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, StatusDegraded, sn.Status)
	require.Equal(t, CodeDNSServerPortInUse, sn.DNSServer.Error.Code)
	require.Contains(t, h.r.list(), "dns.stopserve")
}

func TestPhaseD_KeyUnreadable(t *testing.T) {
	h := newDNSHarness(t)
	h.certs.lanErr = certs.ErrKeyUnreadable
	require.NoError(t, h.o.Connect(context.Background()))
	sn := h.o.Snapshot()
	require.Equal(t, StatusDegraded, sn.Status)
	require.Equal(t, CodeCertKeyUnreadable, sn.DNSServer.Error.Code)
	require.NotContains(t, h.r.list(), "dns.serve")
}

func TestPhaseD_IPChangeRestarts(t *testing.T) {
	h := newDNSHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	n := len(h.srv.runs)
	h.o.checkDNSHealth(context.Background())
	require.Len(t, h.srv.runs, n, "no change, no restart")
	h.setLAN("192.168.1.6")
	h.o.checkDNSHealth(context.Background())
	require.Len(t, h.srv.runs, n+1)
	require.Contains(t, h.srv.last().DoH, netip.MustParseAddrPort("192.168.1.6:443"))
	require.NoError(t, h.srv.last().Cert().Leaf.VerifyHostname("192.168.1.6"))
}

func TestDisconnect_OrderWithDNSServer(t *testing.T) {
	h := newDNSHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	require.NoError(t, h.o.Disconnect(context.Background()))
	calls := h.r.list()
	requireOrder(t, calls, "dns.stopserve", "proxy.stop", "dns.restore", "engine.stop")
	require.Contains(t, calls, "firewall.delete:"+firewall.RuleDNSUDP)
	st, _ := h.states.Load()
	require.Nil(t, st.Firewall)
	require.False(t, h.o.Snapshot().DNSServer.Running)
}

// A restart that fails once (address not ready yet after a Wi-Fi switch)
// is retried: after a minute, or at once when the LAN addresses change.
func TestPhaseD_RetriesAfterFailure(t *testing.T) {
	h := newDNSHarness(t)
	now := time.Now()
	h.o.d.Now = func() time.Time { return now }
	h.srv.err = engine.ErrNoLoopbackDoH
	require.NoError(t, h.o.Connect(context.Background()))
	require.Equal(t, StatusDegraded, h.o.Snapshot().Status)
	h.srv.err = nil
	n := len(h.srv.runs)

	h.o.checkDNSHealth(context.Background())
	require.Len(t, h.srv.runs, n, "not before the retry delay")

	now = now.Add(61 * time.Second)
	h.o.checkDNSHealth(context.Background())
	require.Len(t, h.srv.runs, n+1)
	require.True(t, h.o.Snapshot().DNSServer.Running)
	require.Equal(t, StatusProtected, h.o.Snapshot().Status)
}

func TestPhaseD_RetriesWhenLANChanges(t *testing.T) {
	h := newDNSHarness(t)
	h.srv.err = engine.ErrNoLoopbackDoH
	require.NoError(t, h.o.Connect(context.Background()))
	h.srv.err = nil
	n := len(h.srv.runs)
	h.setLAN("192.168.1.7")
	h.o.checkDNSHealth(context.Background())
	require.Len(t, h.srv.runs, n+1)
}

func TestPhaseD_NoRetryWhenDisabled(t *testing.T) {
	h := newDNSHarness(t)
	h.srv.err = engine.ErrNoLoopbackDoH
	require.NoError(t, h.o.Connect(context.Background()))
	h.settings.DNSServer.Enabled = false
	h.o.d.Now = func() time.Time { return time.Now().Add(time.Hour) }
	n := len(h.srv.runs)
	h.o.checkDNSHealth(context.Background())
	require.Len(t, h.srv.runs, n)
}
