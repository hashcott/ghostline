package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/hashcott/ghostline/internal/model"
	"log/slog"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/procs"
	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/sysproxy"
	"github.com/stretchr/testify/require"
)

type recFirewall struct {
	firewall.Unsupported
	deleted []string
}

func (f *recFirewall) DeleteNamed(name string) error { f.deleted = append(f.deleted, name); return nil }

type recProcs struct {
	procs.Unsupported
	asked []uint32
}

func (p *recProcs) Alive(pid uint32, _ time.Time) bool { p.asked = append(p.asked, pid); return true }

type recProxy struct {
	cur model.WinINETProxy
	set []model.WinINETProxy
}

func (p *recProxy) Query() (model.WinINETProxy, error) { return p.cur, nil }
func (p *recProxy) Set(s model.WinINETProxy) error     { p.set = append(p.set, s); return nil }

func selfSigned(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return der
}

// Review Focus 1: the GUI's startup restore and every headless recovery
// mode use this builder; each hook must reach this OS's implementation.
func TestRecovery_WiresEveryCleanup(t *testing.T) {
	fw, pr := &recFirewall{}, &recProcs{}
	px := &recProxy{cur: model.WinINETProxy{Flags: sysproxy.FlagProxy, Server: "127.0.0.1:8080"}}
	roots := certstore.NewFake()
	session, lan := selfSigned(t, certs.SessionPrefix+" 1"), selfSigned(t, "Ghostline LAN CA")
	require.NoError(t, roots.Install(session))
	require.NoError(t, roots.Install(lan))
	stopped := false
	p := Deps{DNS: sysdns.UnsupportedBackend{}, SysProxy: sysproxy.NewWinINET(px, nil), Certs: roots, Firewall: fw, Procs: pr}

	d := p.Recovery(store.NewStateStore(filepath.Join(t.TempDir(), "state.json"), nil), func() error { stopped = true; return nil }, slog.Default())

	require.NotNil(t, d.DNS)
	require.NoError(t, d.StopDPI())
	require.True(t, stopped)
	require.True(t, d.Alive(42, time.Time{}))
	require.Equal(t, []uint32{42}, pr.asked)
	require.NoError(t, d.DeleteRule(firewall.RuleSetup))
	require.Equal(t, []string{firewall.RuleSetup}, fw.deleted)
	restored, err := d.RestoreSysProxy("127.0.0.1:8080", model.ProxySnapshot{Backend: "windows", Windows: &model.WinINETProxy{Flags: sysproxy.FlagDirect}})
	require.NoError(t, err)
	require.True(t, restored)
	require.Equal(t, []model.WinINETProxy{{Flags: sysproxy.FlagDirect}}, px.set)

	// state.json is user-writable: RemoveCert must refuse anything but a Fake SNI root.
	require.NoError(t, d.RemoveCert(certstore.Thumbprint(lan)))
	require.True(t, roots.Has(certstore.Thumbprint(lan)))
	require.NoError(t, d.RemoveCert(certstore.Thumbprint(session)))
	require.False(t, roots.Has(certstore.Thumbprint(session)))

	require.NoError(t, roots.Install(session))
	require.NoError(t, d.SweepSession(nil))
	require.False(t, roots.Has(certstore.Thumbprint(session)))
	require.True(t, roots.Has(certstore.Thumbprint(lan)))
}
