package app

import (
	"context"
	"errors"
	"github.com/hashcott/ghostline/internal/certstore"
	"slices"
	"strings"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/store"
)

// FakeSNIWarningVersion is the version of the mandatory Fake SNI warning;
// bump it when its content changes so users read it again.
const FakeSNIWarningVersion = 1

const reasonFakeSNI = "fakesni"

// sniState is what the Fake SNI phase changed; guarded by opMu.
type sniState struct {
	issuer    *certs.Issuer
	installed []string // session CA thumbprints in the Root store
	domains   []string
}

func (o *Orchestrator) sniDomains() []string {
	if o.d.Rules == nil {
		return nil
	}
	if c := o.d.Rules(); c != nil {
		return c.SNIDomains()
	}
	return nil
}

// startSNIPhase runs S0–S4 (spec 2B 6.2) after the proxy and DNS server
// phases. A failure undoes the S steps and marks the connection degraded;
// DNS is never touched. Callers hold opMu.
func (o *Orchestrator) startSNIPhase(ctx context.Context) error {
	fs := o.d.Settings().FakeSNI
	domains := o.sniDomains()
	if o.d.Certs == nil || o.d.SetMITM == nil || !fs.Enabled || fs.AckVersion < FakeSNIWarningVersion || len(domains) == 0 {
		o.update(func(sn *Snapshot) { sn.FakeSNI = FakeSNIStatus{} })
		o.clearReason(reasonFakeSNI)
		return nil
	}
	if !o.px.running {
		o.update(func(sn *Snapshot) {
			sn.FakeSNI = FakeSNIStatus{NeedsProxy: true, Error: &AppError{Code: CodeFakeSNINeedsProxy}}
		})
		o.clearReason(reasonFakeSNI)
		return nil
	}
	var ca *certs.CA
	steps := []step{
		{name: "sni.ca", do: func(context.Context) error {
			var err error
			if ca, err = certs.NewSessionCA(domains, o.d.Now()); err != nil {
				if errors.Is(err, certs.ErrTooManyDomains) {
					return appErr(CodeFakeSNITooMany, err, "count", len(domains))
				}
				return appErr(CodeCertInstallFailed, err, "kind", "session")
			}
			return nil
		}},
		{name: "sni.state", do: func(context.Context) error {
			// Write-ahead: the thumbprint is recorded before the CA exists
			// in Root, so every recovery layer can remove it.
			if err := ignoreNoChange(o.setState(func(st *store.State) { st.AddSessionCert(ca.Thumbprint()) })); err != nil {
				return appErr(CodeCertInstallFailed, err, "kind", "session")
			}
			return nil
		}, undo: func(context.Context) error {
			if slices.Contains(o.sni.installed, ca.Thumbprint()) {
				return nil // still in Root: keep it recorded
			}
			return ignoreNoChange(o.setState(func(st *store.State) { st.RemoveSessionCert(ca.Thumbprint()) }))
		}},
		{name: "sni.install", do: func(context.Context) error {
			if err := o.installSession(ca.DER); err != nil {
				o.undoFailedInstall(ca.Thumbprint())
				return appErr(CodeCertInstallFailed, err, "kind", "session")
			}
			o.sni.installed = append(o.sni.installed, ca.Thumbprint())
			return nil
		}, undo: func(context.Context) error {
			return o.removeSessionCA(ca.Thumbprint())
		}},
		{name: "sni.activate", do: func(ctx context.Context) error {
			o.sni.issuer = certs.NewIssuer(ca, o.d.Now)
			o.d.SetMITM(o.sni.issuer)
			if o.d.MITMSelfTest != nil {
				if err := o.d.MITMSelfTest(ctx); err != nil {
					// runSteps undoes only earlier steps: clear this one here.
					o.d.SetMITM(nil)
					o.sni.issuer = nil
					return appErr(CodeFakeSNISelfTest, err)
				}
			}
			return nil
		}, undo: func(context.Context) error {
			o.d.SetMITM(nil)
			o.sni.issuer = nil
			return nil
		}},
	}
	if err := runSteps(ctx, steps, func(int) {}); err != nil {
		var ae *AppError
		if !errors.As(err, &ae) {
			ae = appErr(CodeCertInstallFailed, err, "kind", "session")
		}
		o.update(func(sn *Snapshot) { sn.FakeSNI = FakeSNIStatus{Error: &AppError{Code: ae.Code, Params: ae.Params}} })
		o.addReason(reasonFakeSNI)
		o.log("fakesni", ae.Code, flatten(ae.Params)...)
		return err
	}
	o.sni.domains = domains
	o.setSNIStatus()
	o.clearReason(reasonFakeSNI)
	o.log("fakesni", "FAKESNI_STARTED", "domains", len(domains))
	return nil
}

func (o *Orchestrator) setSNIStatus() {
	ca := o.sni.issuer.CA()
	o.update(func(sn *Snapshot) {
		sn.FakeSNI = FakeSNIStatus{Active: true, Domains: len(o.sni.domains), Thumbprint: ca.Thumbprint(), NotAfter: ca.Cert.NotAfter}
	})
}

// undoFailedInstall removes a CA whose install reported an error (it may
// have been added before a failed read-back). It reports whether the CA is
// gone; if not, it stays in o.sni.installed and recorded in state.json.
func (o *Orchestrator) undoFailedInstall(thumb string) bool {
	if err := o.d.Certs.RemoveSession(thumb); err != nil {
		o.sni.installed = append(o.sni.installed, thumb)
		o.AddWarning(AppError{Code: CodeCertRemoveFailed, Params: map[string]any{"thumbprint": thumb}})
		return false
	}
	return true
}

// forgetSessionCert drops a removed session CA from state.json in any
// phase (after Disconnect the state is clean but may still list one).
func (o *Orchestrator) forgetSessionCert(thumb string) error {
	return o.d.States.Update(func(st *store.State) error {
		st.RemoveSessionCert(thumb)
		return nil
	})
}

// removeSessionCA removes one session CA from Root and, only then, from
// state.json. A failure keeps it recorded for recovery and warns.
func (o *Orchestrator) removeSessionCA(thumb string) error {
	err := o.d.Certs.RemoveSession(thumb)
	var p *certstore.PartialError
	if errors.As(err, &p) {
		// Gone from the system store; an optional one (Firefox, a user's
		// NSS) could not be cleaned. Retrying would not help: warn once.
		o.AddWarning(AppError{Code: CodeCertPartial, Params: map[string]any{"target": strings.Join(p.Targets, ", ")}})
		o.log("fakesni", CodeCertPartial, "target", strings.Join(p.Targets, ", "), "err", p.Err.Error())
		err = nil
	}
	if err != nil {
		o.AddWarning(AppError{Code: CodeCertRemoveFailed, Params: map[string]any{"thumbprint": thumb}})
		o.log("fakesni", CodeCertRemoveFailed, "thumbprint", thumb)
		return err
	}
	o.sni.installed = slices.DeleteFunc(o.sni.installed, func(x string) bool { return x == thumb })
	return o.forgetSessionCert(thumb)
}

// stopSNIPhase turns Fake SNI off and removes every session CA it
// installed. Callers hold opMu.
func (o *Orchestrator) stopSNIPhase(context.Context) {
	if o.d.SetMITM != nil && o.sni.issuer != nil {
		o.d.SetMITM(nil)
	}
	o.sni.issuer, o.sni.domains = nil, nil
	for _, t := range slices.Clone(o.sni.installed) {
		_ = o.removeSessionCA(t)
	}
	o.update(func(sn *Snapshot) { sn.FakeSNI = FakeSNIStatus{} })
	o.clearReason(reasonFakeSNI)
}

// ReapplyFakeSNI restarts the Fake SNI phase after its settings changed.
// It does nothing while not connected.
func (o *Orchestrator) ReapplyFakeSNI(ctx context.Context) error {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return nil
	}
	o.stopSNIPhase(ctx)
	return o.startSNIPhase(ctx)
}

const (
	sniDebounce = 2 * time.Second
	sniRenew    = 3 * 24 * time.Hour
)

// OnRulesCompiled schedules a session CA rotation once rule edits settle
// (spec 2B 5.3): a burst of recompiles rotates once.
func (o *Orchestrator) OnRulesCompiled() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.sniTimer != nil {
		o.sniTimer()
	}
	o.sniTimer = o.d.AfterFunc(sniDebounce, func() {
		o.mu.Lock()
		o.sniTimer = nil
		o.mu.Unlock()
		o.rotateSession(context.Background(), false)
	})
}

// checkSNIHealth rotates the session CA before it expires.
func (o *Orchestrator) checkSNIHealth(ctx context.Context) {
	o.opMu.Lock()
	due := o.sni.issuer != nil && o.sni.issuer.CA().Cert.NotAfter.Sub(o.d.Now()) < sniRenew
	o.opMu.Unlock()
	if due {
		o.rotateSession(ctx, true)
	}
}

// rotateSession replaces the session CA when the sni= domains changed (or
// always, with force): new CA recorded, installed and handed to the proxy
// before the old one is removed. With no CA yet it starts the phase; with
// no domains left it stops it.
func (o *Orchestrator) rotateSession(ctx context.Context, force bool) {
	o.opMu.Lock()
	defer o.opMu.Unlock()
	if st := o.Snapshot().Status; st != StatusProtected && st != StatusDegraded {
		return
	}
	domains := o.sniDomains()
	fs := o.d.Settings().FakeSNI
	switch {
	case o.sni.issuer == nil:
		_ = o.startSNIPhase(ctx)
		return
	case len(domains) == 0 || !fs.Enabled || fs.AckVersion < FakeSNIWarningVersion:
		o.stopSNIPhase(ctx)
		return
	case !force && slices.Equal(domains, o.sni.domains):
		return
	}
	ca, err := certs.NewSessionCA(domains, o.d.Now())
	if err != nil {
		// Too many domains: the phase reports it the same way as at connect.
		o.stopSNIPhase(ctx)
		_ = o.startSNIPhase(ctx)
		return
	}
	thumb := ca.Thumbprint()
	if err := ignoreNoChange(o.setState(func(st *store.State) { st.AddSessionCert(thumb) })); err != nil {
		o.AddWarning(AppError{Code: CodeCertInstallFailed, Params: map[string]any{"kind": "session"}})
		return
	}
	if err := o.installSession(ca.DER); err != nil {
		if o.undoFailedInstall(thumb) {
			_ = ignoreNoChange(o.setState(func(st *store.State) { st.RemoveSessionCert(thumb) }))
		}
		o.AddWarning(AppError{Code: CodeCertInstallFailed, Params: map[string]any{"kind": "session"}})
		o.log("fakesni", CodeCertInstallFailed, "kind", "session")
		return
	}
	o.sni.installed = append(o.sni.installed, thumb)
	old := o.sni.issuer.CA().Thumbprint()
	o.sni.issuer = certs.NewIssuer(ca, o.d.Now)
	o.sni.domains = domains
	o.d.SetMITM(o.sni.issuer)
	o.setSNIStatus()
	_ = o.removeSessionCA(old)
	o.log("fakesni", "FAKESNI_ROTATED", "domains", len(domains))
}

// installSession installs a session CA. On Linux an optional trust store
// (Firefox, a user's NSS) may miss it: Fake SNI still works for the rest,
// so that is a warning, not a failure.
func (o *Orchestrator) installSession(der []byte) error {
	err := o.d.Certs.InstallSession(der)
	var p *certstore.PartialError
	if errors.As(err, &p) {
		if errors.Is(p, certstore.ErrNSSToolMissing) {
			o.AddWarning(AppError{Code: CodeCertNSSToolMissing})
		} else {
			o.AddWarning(AppError{Code: CodeCertPartial, Params: map[string]any{"target": strings.Join(p.Targets, ", ")}})
		}
		o.log("fakesni", CodeCertPartial, "target", strings.Join(p.Targets, ", "), "err", p.Err.Error())
		return nil
	}
	return err
}
