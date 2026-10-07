package core

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/hashcott/ghostline/internal/engine"
	"github.com/miekg/dns"
)

// dnsWiring implements app.DNSServer over the engine.
type dnsWiring struct {
	eng   *engine.Engine
	certs *certWiring

	mu  sync.Mutex
	doh netip.AddrPort // a loopback DoH address being served
}

// Serve implements app.DNSServer.
func (w *dnsWiring) Serve(ctx context.Context, sc engine.ServeConfig) (engine.ServeResult, error) {
	res, err := w.eng.Serve(ctx, sc)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.doh = netip.AddrPort{}
	if err != nil {
		return res, err
	}
	for _, a := range res.Bound {
		if a.Addr().IsLoopback() && a.Addr().Is4() {
			w.doh = a
			break
		}
	}
	return res, nil
}

// StopServe implements app.DNSServer.
func (w *dnsWiring) StopServe(ctx context.Context) error {
	w.mu.Lock()
	w.doh = netip.AddrPort{}
	w.mu.Unlock()
	return w.eng.StopServe(ctx)
}

// SelfTest implements app.DNSServer: a DoH query to loopback, trusting
// only the LAN CA, must reach the engine.
func (w *dnsWiring) SelfTest(ctx context.Context) error {
	w.mu.Lock()
	doh := w.doh
	w.mu.Unlock()
	w.certs.mu.Lock()
	ca := w.certs.lan
	w.certs.mu.Unlock()
	if !doh.IsValid() || ca == nil {
		return errors.New("dns server: not serving DoH on loopback")
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	nonce := "doh-" + hex.EncodeToString(b)
	w.eng.ExpectVerify(nonce)
	q, err := new(dns.Msg).SetQuestion(nonce+"."+engine.VerifySuffix, dns.TypeA).Pack()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+doh.String()+"/dns-query", bytes.NewReader(q))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "dns.ghostline.lan"}}}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("dns server: self test: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK || !w.eng.SawVerify(nonce) {
		return fmt.Errorf("dns server: self test: HTTP %d, query not seen", resp.StatusCode)
	}
	return nil
}
