package sysdns

import (
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/stretchr/testify/require"
)

type modeNM struct {
	fakeNM
	running bool
	mode    string
	modeErr error
	rc      string
}

func (m *modeNM) Running() bool              { return m.running }
func (m *modeNM) DNSMode() (string, error)   { return m.mode, m.modeErr }
func (m *modeNM) RcManager() (string, error) { return m.rc, nil }

type upResolved struct {
	fakeResolved
	running bool
}

func (r *upResolved) Running() bool { return r.running }

func TestDetect_Order(t *testing.T) {
	flush := func() error { return nil }
	fallback := newResolvConf(t.TempDir()+"/resolv.conf", t.TempDir(), flush, nil)
	pick := func(nm nmAPI, r resolvedAPI) Backend { return detect(nm, r, &fakeUnits{}, fallback, flush, nil) }

	b := pick(&modeNM{running: true, mode: "default", rc: "symlink"}, &upResolved{running: true})
	require.Equal(t, "networkmanager", b.Name())
	require.Equal(t, "NetworkManager", b.Info().Chain)

	// Root run (NM 1.58): with dns=systemd-resolved NM keeps its global DNS
	// to itself and resolved never sees it, so resolved is driven directly.
	b = pick(&modeNM{running: true, mode: "systemd-resolved"}, &upResolved{running: true})
	require.Equal(t, "resolved", b.Name())
	require.Equal(t, "systemd-resolved", b.Info().Chain)

	require.Equal(t, "resolved", pick(&modeNM{running: true, mode: "none"}, &upResolved{running: true}).Name())
	require.Equal(t, "resolved", pick(&modeNM{running: true, modeErr: errors.New("old NM")}, &upResolved{running: true}).Name())
	require.Equal(t, "resolved", pick(&modeNM{}, &upResolved{running: true}).Name())
	require.Equal(t, "resolvconf", pick(&modeNM{}, &upResolved{}).Name())
	require.Equal(t, "resolvconf", pick(nil, nil).Name())
}

// Review C1: recovery goes through the backend named in the snapshot, not
// the one detected now (NM restarting, or switched to dns=none since).
func TestDetect_RestoresThroughRecordedBackend(t *testing.T) {
	flush := func() error { return nil }
	fallback := newResolvConf(t.TempDir()+"/resolv.conf", t.TempDir(), flush, nil)
	loop := model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"127.0.0.1"}}}}
	nm := &modeNM{running: true, mode: "none"}
	nm.global = loop
	b := detect(nm, nil, &fakeUnits{}, fallback, flush, nil)
	require.Equal(t, "resolvconf", b.Name())

	s := Snapshot{Backend: "networkmanager", Linux: &model.LinuxDNS{NMGlobal: &model.NMGlobalDNS{}}}
	require.Equal(t, s, b.StillOurs(s), "NM still holds Ghostline's loopback")
	require.Empty(t, b.Restore(s))
	require.Equal(t, model.NMGlobalDNS{}, nm.global)

	// A backend this machine cannot reach keeps the state for a later try.
	b = detect(nil, nil, &fakeUnits{}, fallback, flush, nil)
	require.Equal(t, s, b.StillOurs(s))
	require.NotEmpty(t, b.Restore(s))
}

// Review C1: a corrupt state.json has no snapshot; every backend clears
// what is plainly Ghostline's.
func TestDetect_RestoreDefaultClearsEveryBackend(t *testing.T) {
	flush := func() error { return nil }
	fallback := newResolvConf(t.TempDir()+"/resolv.conf", t.TempDir(), flush, nil)
	nm := &modeNM{running: true, mode: "none"}
	nm.global = model.NMGlobalDNS{Domains: map[string]model.NMDomain{"*": {Servers: []string{"127.0.0.1"}}}}
	b := detect(nm, nil, &fakeUnits{}, fallback, flush, nil)
	require.NoError(t, b.RestoreDefault())
	require.Equal(t, model.NMGlobalDNS{}, nm.global)

	// NM not running: nothing to clear there, and no error.
	require.NoError(t, detect(&modeNM{}, nil, &fakeUnits{}, fallback, flush, nil).RestoreDefault())
}

// Review (minor 6, re-graded): NM with rc-manager=unmanaged does not write
// resolv.conf, so its global DNS would never reach the system.
func TestDetect_SkipsNMThatLeavesResolvConfAlone(t *testing.T) {
	flush := func() error { return nil }
	fallback := newResolvConf(t.TempDir()+"/resolv.conf", t.TempDir(), flush, nil)
	pick := func(nm nmAPI) Backend { return detect(nm, nil, &fakeUnits{}, fallback, flush, nil) }
	require.Equal(t, "resolvconf", pick(&modeNM{running: true, mode: "default", rc: "unmanaged"}).Name())
	require.Equal(t, "resolvconf", pick(&modeNM{running: true, mode: "systemd-resolved", rc: "symlink"}).Name(), "resolved is not running")
	require.Equal(t, "networkmanager", pick(&modeNM{running: true, mode: "default", rc: "symlink"}).Name())
}
