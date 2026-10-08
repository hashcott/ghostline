package engine_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

type lanCert struct {
	ca   *certs.CA
	cur  atomic.Pointer[tls.Certificate]
	pool *x509.CertPool
}

func newLANCert(t *testing.T) *lanCert {
	ca, err := certs.NewLANCA("T", time.Now())
	require.NoError(t, err)
	l := &lanCert{ca: ca, pool: x509.NewCertPool()}
	l.pool.AddCert(ca.Cert)
	l.reissue(t)
	return l
}

func (l *lanCert) reissue(t *testing.T) *tls.Certificate {
	c, err := l.ca.IssueServer([]string{"dns.ghostline.lan"}, []netip.Addr{netip.MustParseAddr("127.0.0.1")}, time.Hour, time.Now())
	require.NoError(t, err)
	l.cur.Store(c)
	return c
}

func serve(t *testing.T, e *engine.Engine, sc engine.ServeConfig) engine.ServeResult {
	t.Helper()
	res, err := e.Serve(context.Background(), sc)
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.StopServe(context.Background()) })
	return res
}

func dohClient(l *lanCert) *http.Client {
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{RootCAs: l.pool}}}
}

func packQuery(t *testing.T, name string, qtype uint16) []byte {
	m := new(dns.Msg).SetQuestion(name, qtype)
	m.Id = 0
	b, err := m.Pack()
	require.NoError(t, err)
	return b
}

func readDNS(t *testing.T, resp *http.Response) *dns.Msg {
	t.Helper()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	m := new(dns.Msg)
	require.NoError(t, m.Unpack(b))
	return m
}

func trapSystemDNS(t *testing.T) {
	old := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		t.Error("system DNS used")
		return nil, errors.New("system DNS forbidden")
	}}
	t.Cleanup(func() { net.DefaultResolver = old })
}

func TestServe_DoHGetPost(t *testing.T) {
	trapSystemDNS(t)
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 80), name: "fake1"}
	e := start(t, nil, up)
	l := newLANCert(t)
	res := serve(t, e, engine.ServeConfig{DoH: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}, Cert: l.cur.Load})
	require.Len(t, res.Bound, 1)
	base := "https://" + res.Bound[0].String() + "/dns-query"
	c := dohClient(l)

	q := packQuery(t, "example.com.", dns.TypeA)
	resp, err := c.Get(base + "?dns=" + base64.RawURLEncoding.EncodeToString(q))
	require.NoError(t, err)
	require.Equal(t, "HTTP/2.0", resp.Proto)
	m := readDNS(t, resp)
	require.Equal(t, "192.0.2.80", m.Answer[0].(*dns.A).A.String())

	resp, err = c.Post(base, "application/dns-message", bytes.NewReader(q))
	require.NoError(t, err)
	m = readDNS(t, resp)
	require.Equal(t, "192.0.2.80", m.Answer[0].(*dns.A).A.String())
	require.Equal(t, uint64(2), e.ServeStats().Queries)
}

// freeDualPort is a loopback port free for both TCP and UDP. TCP is bound
// first: Windows reserves TCP port ranges (Hyper-V), so a port the OS gave
// to UDP may be refused for TCP (WSAEACCES), which broke Swap on CI.
func freeDualPort(t *testing.T) uint16 {
	t.Helper()
	// Not the dynamic range (49152+): Windows runners reserve large parts
	// of it (Hyper-V, WinNAT), for TCP and UDP separately.
	for range 200 {
		port := 20000 + rand.IntN(12000)
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
		l, err := net.Listen("tcp", addr)
		if err != nil {
			continue
		}
		pc, err := net.ListenPacket("udp", addr)
		_ = l.Close()
		if err == nil {
			_ = pc.Close()
			return uint16(port)
		}
	}
	t.Fatal("no port free for both TCP and UDP")
	return 0
}

// servePlain serves plain DNS on a port free for both protocols, trying
// another if it was taken in between.
func servePlain(t *testing.T, e *engine.Engine, l *lanCert) netip.AddrPort {
	t.Helper()
	for range 10 {
		plain := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), freeDualPort(t))
		res, err := e.Serve(context.Background(), engine.ServeConfig{DoH: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}, Plain: []netip.AddrPort{plain}, Cert: l.cur.Load})
		require.NoError(t, err)
		if slices.Contains(res.Bound, plain) {
			t.Cleanup(func() { _ = e.StopServe(context.Background()) })
			return plain
		}
		require.NoError(t, e.StopServe(context.Background()))
	}
	t.Fatal("no free port for plain DNS")
	return netip.AddrPort{}
}

func TestServe_PlainUDPTCP(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 81), name: "fake1"}
	e := start(t, nil, up)
	plain := servePlain(t, e, newLANCert(t))
	for _, netw := range []string{"udp", "tcp"} {
		c := &dns.Client{Net: netw, Timeout: 2 * time.Second}
		r, _, err := c.Exchange(new(dns.Msg).SetQuestion("example.com.", dns.TypeA), plain.String())
		require.NoError(t, err, netw)
		require.Equal(t, "192.0.2.81", r.Answer[0].(*dns.A).A.String(), netw)
	}
}

func TestServe_RefusesANY(t *testing.T) {
	up := &fakeUp{ip: net.IPv4(192, 0, 2, 81), name: "fake1"}
	e := start(t, nil, up)
	plain := servePlain(t, e, newLANCert(t))
	r, _, err := (&dns.Client{Timeout: 2 * time.Second}).Exchange(new(dns.Msg).SetQuestion("example.com.", dns.TypeANY), plain.String())
	require.NoError(t, err)
	require.Equal(t, dns.RcodeRefused, r.Rcode)
	require.Zero(t, up.calls.Load())
}

func TestServe_SkipsBusyAddr(t *testing.T) {
	e := start(t, nil, &fakeUp{ip: net.IPv4(192, 0, 2, 80)})
	l := newLANCert(t)
	busy, err := net.Listen("tcp", "127.0.0.2:0")
	require.NoError(t, err)
	defer busy.Close()
	busyAP := netip.MustParseAddrPort(busy.Addr().String())
	res := serve(t, e, engine.ServeConfig{DoH: []netip.AddrPort{busyAP, netip.MustParseAddrPort("127.0.0.1:0")}, Cert: l.cur.Load})
	require.Contains(t, res.Skipped, busyAP)
	require.Len(t, res.Bound, 1)
}

func TestServe_NoLoopbackBoundFails(t *testing.T) {
	e := start(t, nil, &fakeUp{ip: net.IPv4(192, 0, 2, 80)})
	l := newLANCert(t)
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer busy.Close()
	_, err = e.Serve(context.Background(), engine.ServeConfig{DoH: []netip.AddrPort{netip.MustParseAddrPort(busy.Addr().String())}, Cert: l.cur.Load})
	require.ErrorIs(t, err, engine.ErrNoLoopbackDoH)
}

func TestServe_CertRotates(t *testing.T) {
	e := start(t, nil, &fakeUp{ip: net.IPv4(192, 0, 2, 80)})
	l := newLANCert(t)
	res := serve(t, e, engine.ServeConfig{DoH: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}, Cert: l.cur.Load})
	serial := func() string {
		conn, err := tls.Dial("tcp", res.Bound[0].String(), &tls.Config{RootCAs: l.pool, ServerName: "dns.ghostline.lan"})
		require.NoError(t, err)
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber.String()
	}
	first := serial()
	l.reissue(t)
	require.NotEqual(t, first, serial())
}

// Stopping right after starting must not crash: dnsproxy's own DoH
// goroutine read its server after Shutdown had cleared it (nil Serve).
func TestServe_StopRightAfterStart(t *testing.T) {
	e := start(t, nil, &fakeUp{ip: net.IPv4(192, 0, 2, 80)})
	l := newLANCert(t)
	for range 300 {
		_, err := e.Serve(context.Background(), engine.ServeConfig{DoH: []netip.AddrPort{netip.MustParseAddrPort("127.0.0.1:0")}, Cert: l.cur.Load})
		require.NoError(t, err)
		require.NoError(t, e.StopServe(context.Background()))
	}
}
