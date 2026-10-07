package app

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/store"
)

// SetupPage is the temporary phone setup page (*dnsserver.SetupPage).
type SetupPage interface {
	Start(addrs []netip.AddrPort, life time.Duration) error
	Stop() error
	Running() bool
	Remaining() time.Duration
}

// setupLife is how long the phone setup page stays open (spec 2B 7.2).
const setupLife = 10 * time.Minute

var errDNSNotShared = errors.New("the DNS server is not running for the LAN")

// LANCA returns the LAN CA of the running DNS server, or nil.
func (o *Orchestrator) LANCA() *certs.CA {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	return o.dns.ca
}

// lanFiles builds what the phone setup page and "Save files…" offer.
// The iOS profile is built per Wi-Fi name: the saved one by default, or
// the one entered on the phone.
func lanFiles(ca *certs.CA, lan []netip.Addr, dohPort int, ssid string) (dnsserver.SetupFiles, error) {
	lan = slices.Clone(lan)
	return dnsserver.SetupFiles{
		CRT: ca.DER, Fingerprint: ca.Fingerprint(), SSID: ssid,
		MobileConfig: func(name string) ([]byte, error) {
			return certs.MobileConfig(certs.ProfileInput{CA: ca, Addrs: lan, Port: dohPort, SSID: name})
		},
	}, nil
}

// OpenSetupPage opens the phone setup page on the LAN addresses for ten
// minutes, with its firewall rule recorded in state.json first. It returns
// the URL for the QR code.
func (o *Orchestrator) OpenSetupPage(newPage func(dnsserver.SetupFiles, func()) SetupPage, ssid string) (string, error) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	ds := o.d.Settings().DNSServer
	if !o.dns.running || !ds.ShareLAN || o.dns.ca == nil || len(o.dns.lan) == 0 || newPage == nil {
		return "", appErr(CodeSetupPageFailed, errDNSNotShared)
	}
	o.closeSetup()
	files, err := lanFiles(o.dns.ca, o.dns.lan, ds.DoHPort, ssid)
	if err != nil {
		return "", appErr(CodeSetupPageFailed, err)
	}
	rule := firewall.Rule{Name: firewall.RuleSetup, Protocol: "TCP", Ports: []int{store.SetupPagePort}}
	if o.d.Firewall != nil {
		if err := ignoreNoChange(o.setState(func(st *store.State) { st.AddFirewallRule(rule.Name) })); err != nil {
			return "", appErr(CodeSetupPageFailed, err)
		}
		if err := o.d.Firewall.AddNamed(rule); err != nil {
			o.dropSetupRule()
			return "", appErr(CodeSetupPageFailed, err)
		}
	}
	var page SetupPage
	page = newPage(files, func() {
		o.dropSetupRule()
		o.mu.Lock()
		if o.setup == page {
			o.setup, o.setupURL = nil, ""
		}
		o.mu.Unlock()
	})
	var addrs []netip.AddrPort
	for _, a := range o.dns.lan {
		addrs = append(addrs, netip.AddrPortFrom(a, store.SetupPagePort))
	}
	if err := page.Start(addrs, setupLife); err != nil {
		o.dropSetupRule()
		return "", appErr(CodeSetupPageFailed, err)
	}
	url := "http://" + hostPort(o.dns.lan[0], store.SetupPagePort) + "/"
	o.mu.Lock()
	o.setup, o.setupURL = page, url
	o.mu.Unlock()
	return url, nil
}

// dropSetupRule removes the setup page's firewall rule and its record.
func (o *Orchestrator) dropSetupRule() {
	if o.d.Firewall == nil {
		return
	}
	if err := o.d.Firewall.DeleteNamed(firewall.RuleSetup); err != nil {
		o.log("dnsserver", CodeSetupPageFailed, "detail", err.Error())
		return
	}
	_ = ignoreNoChange(o.setState(func(st *store.State) { st.RemoveFirewallRule(firewall.RuleSetup) }))
}

// closeSetup stops the setup page; its OnStop removes the rule.
func (o *Orchestrator) closeSetup() {
	o.mu.Lock()
	p := o.setup
	o.setup, o.setupURL = nil, ""
	o.mu.Unlock()
	if p != nil {
		_ = p.Stop()
	}
}

// CloseSetupPage closes the phone setup page early.
func (o *Orchestrator) CloseSetupPage() {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	o.closeSetup()
}

// SetupInfo is the open setup page's URL and remaining time.
func (o *Orchestrator) SetupInfo() (string, time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.setup == nil || !o.setup.Running() {
		return "", 0
	}
	return o.setupURL, o.setup.Remaining()
}

func hostPort(a netip.Addr, port int) string {
	return netip.AddrPortFrom(a, uint16(port)).String()
}

// RemoveAllCerts turns the DNS server and Fake SNI phases off, then
// removes every Ghostline root from the system store.
func (o *Orchestrator) RemoveAllCerts(ctx context.Context) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	o.stopSNIPhase(ctx)
	o.stopDNSPhase(ctx)
	if o.d.Certs == nil {
		return nil
	}
	list, err := o.d.Certs.List()
	if err != nil {
		return err
	}
	var errs []error
	lan := false
	for _, c := range list {
		if len(c.Subject) >= len(certs.LANPrefix) && c.Subject[:len(certs.LANPrefix)] == certs.LANPrefix {
			lan = true
			continue
		}
		if err := o.d.Certs.RemoveSession(c.Thumbprint); err != nil {
			errs = append(errs, err)
			continue
		}
		_ = ignoreNoChange(o.setState(func(st *store.State) { st.RemoveSessionCert(c.Thumbprint) }))
	}
	if lan {
		errs = append(errs, o.d.Certs.RemoveLANCA(ctx))
	}
	return errors.Join(errs...)
}

// RetryCertRemoval removes session CAs still recorded in state.json that
// no running session uses (after CERT_REMOVE_FAILED).
func (o *Orchestrator) RetryCertRemoval() error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	st, err := o.d.States.Load()
	if err != nil || o.d.Certs == nil {
		return err
	}
	pending := slices.Clone(o.sni.installed)
	if st.Certs != nil {
		for _, t := range st.Certs.Session {
			if !slices.Contains(pending, t) {
				pending = append(pending, t)
			}
		}
	}
	current := ""
	if o.sni.issuer != nil {
		current = o.sni.issuer.CA().Thumbprint()
	}
	var errs []error
	for _, t := range pending {
		if t == current {
			continue
		}
		if err := o.d.Certs.RemoveSession(t); err != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", t, err))
			continue
		}
		o.sni.installed = slices.DeleteFunc(o.sni.installed, func(x string) bool { return x == t })
		errs = append(errs, o.forgetSessionCert(t))
	}
	if errors.Join(errs...) == nil {
		o.ClearWarning(CodeCertRemoveFailed)
	}
	return errors.Join(errs...)
}

// listCerts is the Root store content for the certificates section.
func (o *Orchestrator) listCerts() ([]certstore.Cert, error) {
	if o.d.Certs == nil {
		return []certstore.Cert{}, nil
	}
	l, err := o.d.Certs.List()
	if l == nil {
		l = []certstore.Cert{}
	}
	return l, err
}
