package app

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/netip"
	"slices"
	"sync/atomic"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/dnsserver"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/store"
)

const (
	reasonDNSServer = "dnsserver"
	dohLeafLife     = 90 * 24 * time.Hour
	dohLeafRenew    = 14 * 24 * time.Hour
	dohName         = "dns." + certs.LANDomain
	dnsRetryAfter   = time.Minute
)

// dnsState is what the DNS server phase changed; guarded by opMu.
type dnsState struct {
	running bool
	fw      []string // firewall rules created
	lan     []netip.Addr
	ca      *certs.CA
	leaf    atomic.Pointer[tls.Certificate]
	// failedAt/failedLAN describe the last failed start, for retries.
	failedAt  time.Time
	failedLAN []netip.Addr
}

// dnsRules are the inbound rules for sharing the DNS server on the LAN.
func dnsRules(dohPort int) []firewall.Rule {
	return []firewall.Rule{
		{Name: firewall.RuleDNSTCP, Protocol: "TCP", Ports: []int{53, dohPort}},
		{Name: firewall.RuleDNSUDP, Protocol: "UDP", Ports: []int{53}},
	}
}

// setState applies fn to state.json while DNS is set (the 2B parts are
// written only then, so every recovery layer sees them).
func (o *Orchestrator) setState(fn func(st *store.State)) error { return o.setSysProxyState(fn) }

// issueDoHLeaf signs the DoH certificate for the loopback and LAN
// addresses this run listens on.
func (o *Orchestrator) issueDoHLeaf(ca *certs.CA, lan []netip.Addr) error {
	ips := append([]netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.IPv6Loopback()}, lan...)
	leaf, err := ca.IssueServer([]string{dohName}, ips, dohLeafLife, o.d.Now())
	if err != nil {
		return err
	}
	o.dns.leaf.Store(leaf)
	return nil
}

func (o *Orchestrator) dnsLeaf() *tls.Certificate { return o.dns.leaf.Load() }

func (o *Orchestrator) lanAddrs(share bool) []netip.Addr {
	if !share || o.d.LANAddrs == nil {
		return nil
	}
	return o.d.LANAddrs()
}

// startDNSPhase runs D1–D3 (spec 2B 6.1) after the proxy phase. A failure
// undoes the D steps and marks the connection degraded; DNS is never
// touched. Callers hold opMu.
func (o *Orchestrator) startDNSPhase(ctx context.Context) error {
	s := o.d.Settings()
	ds := s.DNSServer
	if o.d.DNSServer == nil || o.d.Certs == nil || !ds.Enabled {
		o.update(func(sn *Snapshot) { sn.DNSServer = DNSServerStatus{} })
		o.clearReason(reasonDNSServer)
		return nil
	}
	o.mu.Lock()
	v6 := o.v6
	o.mu.Unlock()
	lan := o.lanAddrs(ds.ShareLAN)
	doh, plain := dnsserver.ListenPlan(lan, ds.DoHPort, ds.ShareLAN, v6)
	var res engine.ServeResult

	steps := []step{
		{name: "dns.lanca", do: func(ctx context.Context) error {
			ca, err := o.d.Certs.LANCA(ctx)
			if err != nil {
				if errors.Is(err, certs.ErrKeyUnreadable) {
					return appErr(CodeCertKeyUnreadable, err)
				}
				return appErr(CodeCertInstallFailed, err, "kind", "lan")
			}
			o.dns.ca, o.dns.lan = ca, lan
			if err := o.issueDoHLeaf(ca, lan); err != nil {
				return appErr(CodeCertInstallFailed, err, "kind", "lan")
			}
			return nil
		}},
		{name: "dns.firewall", do: func(context.Context) error {
			if !ds.ShareLAN || o.d.Firewall == nil {
				return nil
			}
			// Block Public before anything listens on the LAN.
			if err := o.ensureBlockPublic(); err != nil {
				return appErr(CodeDNSServerFirewall, err, "detail", err.Error())
			}
			for _, r := range dnsRules(ds.DoHPort) {
				if err := ignoreNoChange(o.setState(func(st *store.State) { st.AddFirewallRule(r.Name) })); err != nil {
					return appErr(CodeDNSServerFirewall, err, "detail", err.Error())
				}
				o.dns.fw = append(o.dns.fw, r.Name)
				if err := o.d.Firewall.AddNamed(r); err != nil {
					return appErr(CodeDNSServerFirewall, err, "detail", err.Error())
				}
			}
			return nil
		}, undo: func(context.Context) error {
			return o.deleteDNSRules()
		}},
		{name: "dns.serve", do: func(ctx context.Context) error {
			var err error
			res, err = o.d.DNSServer.Serve(ctx, engine.ServeConfig{DoH: doh, Plain: plain, Cert: o.dnsLeaf})
			if err == nil && len(plain) > 0 && !anyBound(res.Bound, plain) {
				warnIgnored("dnsserver stop", o.d.DNSServer.StopServe(context.WithoutCancel(ctx)))
				err = errors.New("no LAN address could be bound")
			}
			if err != nil {
				return appErr(CodeDNSServerPortInUse, err, "port", ds.DoHPort, "addrs", skippedList(res))
			}
			o.dns.running = true
			if err := o.d.DNSServer.SelfTest(ctx); err != nil {
				return appErr(CodeDNSServerSelfTest, err)
			}
			return nil
		}, undo: func(ctx context.Context) error {
			o.dns.running = false
			return o.d.DNSServer.StopServe(ctx)
		}},
	}
	if err := runSteps(ctx, steps, func(int) {}); err != nil {
		var ae *AppError
		if !errors.As(err, &ae) {
			ae = appErr(CodeDNSServerPortInUse, err)
		}
		if o.dns.running {
			o.dns.running = false
			warnIgnored("dnsserver stop", o.d.DNSServer.StopServe(context.WithoutCancel(ctx)))
		}
		warnIgnored("dnsserver firewall delete", o.deleteDNSRules())
		o.update(func(sn *Snapshot) {
			sn.DNSServer = DNSServerStatus{Error: &AppError{Code: ae.Code, Params: ae.Params}, Skipped: skippedMap(res)}
		})
		o.addReason(reasonDNSServer)
		o.dns.failedAt, o.dns.failedLAN = o.d.Now(), lan
		o.logAppErr("dnsserver", ae)
		return err
	}
	o.dns.failedAt, o.dns.failedLAN = time.Time{}, nil
	o.update(func(sn *Snapshot) {
		sn.DNSServer = DNSServerStatus{Running: true, Addrs: boundDoH(res.Bound, doh), Skipped: skippedMap(res)}
	})
	o.clearReason(reasonDNSServer)
	o.log("dnsserver", "DNSSERVER_STARTED", "addrs", len(res.Bound))
	return nil
}

// deleteDNSRules removes the DNS firewall rules this phase created and
// forgets them in state.json.
func (o *Orchestrator) deleteDNSRules() error {
	var errs []error
	for _, name := range o.dns.fw {
		if o.d.Firewall != nil {
			if err := o.d.Firewall.DeleteNamed(name); err != nil {
				errs = append(errs, err)
				continue
			}
		}
		errs = append(errs, ignoreNoChange(o.setState(func(st *store.State) { st.RemoveFirewallRule(name) })))
	}
	o.dns.fw = nil
	return errors.Join(errs...)
}

// stopDNSPhase undoes the DNS server phase. Callers hold opMu.
func (o *Orchestrator) stopDNSPhase(ctx context.Context) {
	o.closeSetup()
	if o.dns.running {
		warnIgnored("dnsserver stop", o.d.DNSServer.StopServe(ctx))
	}
	if err := o.deleteDNSRules(); err != nil {
		o.logErr("dnsserver", CodeDNSServerFirewall, err, "detail", err.Error())
	}
	o.dns.running, o.dns.lan, o.dns.ca = false, nil, nil
	o.update(func(sn *Snapshot) { sn.DNSServer = DNSServerStatus{} })
	o.clearReason(reasonDNSServer)
}

// ReapplyDNSServer restarts the DNS server phase after its settings
// changed. It does nothing while not connected.
func (o *Orchestrator) ReapplyDNSServer(ctx context.Context) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return nil
	}
	o.stopDNSPhase(ctx)
	return o.startDNSPhase(ctx)
}

// checkDNSHealth follows LAN address changes (Wi-Fi switch) and renews the
// DoH certificate before it expires.
func (o *Orchestrator) checkDNSHealth(ctx context.Context) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	ds := o.d.Settings().DNSServer
	if !o.dns.running {
		// Failed start (spec 2B 11: retry, then stay degraded): try again
		// after a minute, or at once when the LAN addresses change.
		st := o.Snapshot().Status
		connected := st == StatusProtected || st == StatusDegraded
		if !connected || !ds.Enabled || o.dns.failedAt.IsZero() {
			return
		}
		if o.d.Now().Sub(o.dns.failedAt) >= dnsRetryAfter || !sameAddrs(o.lanAddrs(ds.ShareLAN), o.dns.failedLAN) {
			o.log("dnsserver", "DNSSERVER_RESTART")
			_ = o.startDNSPhase(ctx)
		}
		return
	}
	if lan := o.lanAddrs(ds.ShareLAN); !sameAddrs(lan, o.dns.lan) {
		o.log("dnsserver", "DNSSERVER_RESTART")
		o.stopDNSPhase(ctx)
		_ = o.startDNSPhase(ctx)
		return
	}
	if leaf := o.dns.leaf.Load(); leaf != nil && o.dns.ca != nil && leaf.Leaf.NotAfter.Sub(o.d.Now()) < dohLeafRenew {
		if err := o.issueDoHLeaf(o.dns.ca, o.dns.lan); err != nil {
			slog.Warn("dnsserver: DoH certificate renewal failed", "err", err)
		}
	}
}

func sameAddrs(a, b []netip.Addr) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, func(x, y netip.Addr) int { return x.Compare(y) })
	slices.SortFunc(b, func(x, y netip.Addr) int { return x.Compare(y) })
	return slices.Equal(a, b)
}

func anyBound(bound, want []netip.AddrPort) bool {
	for _, w := range want {
		if slices.Contains(bound, w) {
			return true
		}
	}
	return false
}

func boundDoH(bound, doh []netip.AddrPort) []string {
	var out []string
	for _, a := range doh {
		if slices.Contains(bound, a) {
			out = append(out, a.String())
		}
	}
	return out
}

func skippedMap(res engine.ServeResult) map[string]string {
	if len(res.Skipped) == 0 {
		return nil
	}
	m := make(map[string]string, len(res.Skipped))
	for a, why := range res.Skipped {
		m[a.String()] = why
	}
	return m
}

func skippedList(res engine.ServeResult) []string {
	var out []string
	for a := range res.Skipped {
		out = append(out, a.String())
	}
	slices.Sort(out)
	return out
}

// ensureBlockPublic adds the Public-profile block rule once (recorded in
// state.json first), so Allow rules Windows creates from its firewall
// prompt can never open LAN sharing on a public network.
func (o *Orchestrator) ensureBlockPublic() error {
	if o.blockPublic || o.d.Firewall == nil {
		return nil
	}
	if err := ignoreNoChange(o.setState(func(st *store.State) { st.AddFirewallRule(firewall.RuleBlockPublic) })); err != nil {
		return err
	}
	if err := o.d.Firewall.AddNamed(firewall.BlockPublicRule); err != nil {
		warnIgnored("state update", ignoreNoChange(o.setState(func(st *store.State) { st.RemoveFirewallRule(firewall.RuleBlockPublic) })))
		return err
	}
	o.blockPublic = true
	return nil
}

// dropBlockPublic removes the block rule at Disconnect.
func (o *Orchestrator) dropBlockPublic() {
	if !o.blockPublic {
		return
	}
	if err := o.d.Firewall.DeleteNamed(firewall.RuleBlockPublic); err != nil {
		o.logErr("proxy", CodeProxyFirewall, err, "detail", err.Error())
		return // stays recorded: recovery removes it
	}
	o.blockPublic = false
	warnIgnored("state update", ignoreNoChange(o.setState(func(st *store.State) { st.RemoveFirewallRule(firewall.RuleBlockPublic) })))
}
