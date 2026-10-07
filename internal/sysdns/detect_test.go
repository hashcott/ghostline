package sysdns

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type modeNM struct {
	fakeNM
	running bool
	mode    string
	modeErr error
}

func (m *modeNM) Running() bool            { return m.running }
func (m *modeNM) DNSMode() (string, error) { return m.mode, m.modeErr }

type upResolved struct {
	fakeResolved
	running bool
}

func (r *upResolved) Running() bool { return r.running }

func TestDetect_Order(t *testing.T) {
	flush := func() error { return nil }
	fallback := newResolvConf(t.TempDir()+"/resolv.conf", t.TempDir(), flush, nil)
	pick := func(nm nmAPI, r resolvedAPI) Backend { return detect(nm, r, &fakeUnits{}, fallback, flush, nil) }

	b := pick(&modeNM{running: true, mode: "default"}, &upResolved{running: true})
	require.Equal(t, "networkmanager", b.Name())
	require.Equal(t, "NetworkManager → systemd-resolved", b.Info().Chain)
	require.Equal(t, "NetworkManager", pick(&modeNM{running: true, mode: "default"}, &upResolved{}).Info().Chain)

	require.Equal(t, "resolved", pick(&modeNM{running: true, mode: "none"}, &upResolved{running: true}).Name())
	require.Equal(t, "resolved", pick(&modeNM{running: true, modeErr: errors.New("old NM")}, &upResolved{running: true}).Name())
	require.Equal(t, "resolved", pick(&modeNM{}, &upResolved{running: true}).Name())
	require.Equal(t, "resolvconf", pick(&modeNM{}, &upResolved{}).Name())
	require.Equal(t, "resolvconf", pick(nil, nil).Name())
}
