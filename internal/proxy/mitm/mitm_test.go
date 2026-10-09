package mitm_test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/proxy/mitm"
	"github.com/hashcott/ghostline/internal/tlsfrag"
	"github.com/stretchr/testify/require"
)

// publicCA stands in for the system roots: an unconstrained test CA that
// signs the "real" server certificates.
type publicCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newPublicCA(t *testing.T) *publicCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Public Root"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &publicCA{cert: cert, key: key, pool: pool}
}

func (p *publicCA) leaf(t *testing.T, names []string, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: names[0]},
		DNSNames: names, NotBefore: time.Now().Add(-2 * time.Hour), NotAfter: notAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.cert, &key.PublicKey, p.key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// server is the CDN edge: it records the SNI it was asked for.
type server struct {
	srv *httptest.Server
	mu  sync.Mutex
	sni []string
}

func (s *server) gotSNI() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sni...)
}

func newServer(t *testing.T, cert tls.Certificate, protos []string, abort bool) *server {
	t.Helper()
	s := &server{}
	s.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "hello from "+r.Host+" over "+r.Proto)
	}))
	s.srv.EnableHTTP2 = true
	s.srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   protos,
		GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			s.mu.Lock()
			s.sni = append(s.sni, chi.ServerName)
			s.mu.Unlock()
			if abort {
				return nil, errors.New("unknown SNI")
			}
			return nil, nil
		},
	}
	s.srv.StartTLS()
	if protos != nil {
		s.srv.TLS.NextProtos = protos // StartTLS adds h2/http1.1 defaults
	} else {
		s.srv.TLS.NextProtos = nil
	}
	t.Cleanup(s.srv.Close)
	return s
}

func readHello(t *testing.T, br *bufio.Reader) []byte {
	t.Helper()
	hdr, err := br.Peek(5)
	require.NoError(t, err)
	n, _ := tlsfrag.RecordLen(hdr)
	hello := make([]byte, n)
	_, err = io.ReadFull(br, hello)
	require.NoError(t, err)
	return hello
}

type fixture struct {
	pub     *publicCA
	session *certs.CA
	issuer  *certs.Issuer
}

func newFixture(t *testing.T) *fixture {
	pub := newPublicCA(t)
	ses, err := certs.NewSessionCA([]string{"youtube.com"}, time.Now())
	require.NoError(t, err)
	return &fixture{pub: pub, session: ses, issuer: certs.NewIssuer(ses, time.Now)}
}

// proxyOnce runs one Fake SNI interception on a loopback listener and
// returns its address and a channel with the first error.
func (f *fixture) proxyOnce(t *testing.T, upstream string, fakeSNI string) (string, chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { ln.Close() })
	errc := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errc <- err
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		hello := readHello(t, br)
		raw, err := net.Dial("tcp", upstream)
		if err != nil {
			errc <- err
			return
		}
		p := mitm.Params{Host: "youtube.com", FakeSNI: fakeSNI, Hello: hello, Roots: f.pub.pool, Timeout: 5 * time.Second, Now: time.Now}
		ctx := context.Background()
		sc, err := mitm.DialServer(ctx, raw, p)
		if err != nil {
			raw.Close()
			errc <- err
			return
		}
		defer sc.Close()
		cc, err := mitm.AcceptClient(ctx, c, br, p, f.issuer, sc.ConnectionState().NegotiatedProtocol)
		if err != nil {
			errc <- err
			return
		}
		errc <- nil
		go func() { _, _ = io.Copy(sc, cc) }()
		_, _ = io.Copy(cc, sc)
	}()
	return ln.Addr().String(), errc
}

func (f *fixture) client(proxyAddr string, trustSession bool, protos []string) *http.Client {
	pool := x509.NewCertPool()
	if trustSession {
		pool.AddCert(f.session.Cert)
	}
	return &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   &tls.Config{RootCAs: pool, NextProtos: protos},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, proxyAddr)
		},
	}}
}

func get(t *testing.T, c *http.Client) (string, error) {
	t.Helper()
	resp, err := c.Get("https://youtube.com/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func TestMITM_SendsFakeSNI(t *testing.T) {
	f := newFixture(t)
	srv := newServer(t, f.pub.leaf(t, []string{"www.google.com"}, time.Now().Add(time.Hour)), []string{"h2", "http/1.1"}, false)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "www.google.com")
	body, err := get(t, f.client(addr, true, nil))
	require.NoError(t, err)
	require.NoError(t, <-errc)
	require.Equal(t, "hello from youtube.com over HTTP/2.0", body)
	require.Equal(t, []string{"www.google.com"}, srv.gotSNI())
}

func TestMITM_NoSNI(t *testing.T) {
	f := newFixture(t)
	srv := newServer(t, f.pub.leaf(t, []string{"youtube.com"}, time.Now().Add(time.Hour)), []string{"http/1.1"}, false)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "")
	body, err := get(t, f.client(addr, true, nil))
	require.NoError(t, err)
	require.NoError(t, <-errc)
	require.Equal(t, "hello from youtube.com over HTTP/1.1", body)
	require.Equal(t, []string{""}, srv.gotSNI())
}

func TestMITM_VerifyAcceptsFakeOrReal(t *testing.T) {
	cases := []struct {
		name    string
		certFor []string
		fake    string
		expired bool
		wantErr error
	}{
		{"fake name", []string{"www.google.com"}, "www.google.com", false, nil},
		{"real host", []string{"youtube.com"}, "www.google.com", false, nil},
		{"other name", []string{"other.com"}, "www.google.com", false, mitm.ErrVerifyFailed},
		{"expired", []string{"www.google.com"}, "www.google.com", true, mitm.ErrVerifyFailed},
		{"no SNI needs real host", []string{"www.google.com"}, "", false, mitm.ErrVerifyFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			notAfter := time.Now().Add(time.Hour)
			if tc.expired {
				notAfter = time.Now().Add(-time.Minute)
			}
			srv := newServer(t, f.pub.leaf(t, tc.certFor, notAfter), []string{"http/1.1"}, false)
			addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), tc.fake)
			go func() { _, _ = get(t, f.client(addr, true, nil)) }()
			err := <-errc
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestMITM_UntrustedServerRoot(t *testing.T) {
	f := newFixture(t)
	other := newPublicCA(t)
	srv := newServer(t, other.leaf(t, []string{"www.google.com"}, time.Now().Add(time.Hour)), []string{"http/1.1"}, false)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "www.google.com")
	go func() { _, _ = get(t, f.client(addr, true, nil)) }()
	require.ErrorIs(t, <-errc, mitm.ErrVerifyFailed)
}

func TestMITM_ServerAlert(t *testing.T) {
	f := newFixture(t)
	srv := newServer(t, f.pub.leaf(t, []string{"www.google.com"}, time.Now().Add(time.Hour)), []string{"http/1.1"}, true)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "www.google.com")
	go func() { _, _ = get(t, f.client(addr, true, nil)) }()
	require.ErrorIs(t, <-errc, mitm.ErrServerRejected)
}

func TestMITM_ALPN(t *testing.T) {
	f := newFixture(t)
	srv := newServer(t, f.pub.leaf(t, []string{"www.google.com"}, time.Now().Add(time.Hour)), []string{"http/1.1"}, false)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "www.google.com")
	body, err := get(t, f.client(addr, true, nil))
	require.NoError(t, err)
	require.NoError(t, <-errc)
	require.Equal(t, "hello from youtube.com over HTTP/1.1", body)
}

func TestMITM_ALPNServerChoosesNone(t *testing.T) {
	f := newFixture(t)
	srv := newServer(t, f.pub.leaf(t, []string{"www.google.com"}, time.Now().Add(time.Hour)), nil, false)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "www.google.com")
	// The client offers h2; the server picks nothing, so h2 must not be
	// negotiated towards the client either.
	body, err := get(t, f.client(addr, true, []string{"h2", "http/1.1"}))
	require.NoError(t, err)
	require.NoError(t, <-errc)
	require.Equal(t, "hello from youtube.com over HTTP/1.1", body)
}

func TestMITM_ClientRejects(t *testing.T) {
	f := newFixture(t)
	srv := newServer(t, f.pub.leaf(t, []string{"www.google.com"}, time.Now().Add(time.Hour)), []string{"http/1.1"}, false)
	addr, errc := f.proxyOnce(t, srv.srv.Listener.Addr().String(), "www.google.com")
	_, err := get(t, f.client(addr, false, nil))
	require.Error(t, err)
	require.ErrorIs(t, <-errc, mitm.ErrClientRejected)
}
