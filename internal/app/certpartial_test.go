package app

import (
	"context"
	"errors"
	"testing"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/stretchr/testify/require"
)

// Linux: the system store took the CA but Firefox's policy could not be
// written. Fake SNI still runs; the user is told which browser misses it.
func TestPhaseS_PartialInstallWarns(t *testing.T) {
	h := newSNIHarness(t)
	h.certs.partial = &certstore.PartialError{Targets: []string{"firefox"}, Err: errors.New("read-only")}
	require.NoError(t, h.o.Connect(context.Background()))
	require.NotNil(t, h.active(), "Fake SNI is active")
	var found bool
	for _, w := range h.o.Snapshot().Warnings {
		if w.Code == CodeCertPartial {
			found = true
			require.Equal(t, "firefox", w.Params["target"])
		}
	}
	require.True(t, found)
	require.NotContains(t, warningCodes(h.o.Snapshot()), CodeCertInstallFailed)
}

// Ubuntu without libnss3-tools: Chrome misses the root; say what to install.
func TestPhaseS_NSSToolMissingWarns(t *testing.T) {
	h := newSNIHarness(t)
	h.certs.partial = &certstore.PartialError{Targets: []string{"nss"}, Err: certstore.ErrNSSToolMissing}
	require.NoError(t, h.o.Connect(context.Background()))
	require.Contains(t, warningCodes(h.o.Snapshot()), CodeCertNSSToolMissing)
	require.NotContains(t, warningCodes(h.o.Snapshot()), CodeCertPartial)
}

// Review I5: the system store is clean but Firefox's policy could not be
// edited (malformed). The CA is gone from where it mattered: forget it,
// warn, and do not retry forever.
func TestPhaseS_PartialRemovalForgetsTheCA(t *testing.T) {
	h := newSNIHarness(t)
	require.NoError(t, h.o.Connect(context.Background()))
	h.certs.removePartial = &certstore.PartialError{Targets: []string{"firefox"}, Err: errors.New("malformed policies.json")}
	require.NoError(t, h.o.Disconnect(context.Background()))
	st, err := h.states.Load()
	require.NoError(t, err)
	require.Nil(t, st.Certs, "nothing left to retry")
	require.NotContains(t, warningCodes(h.o.Snapshot()), CodeCertRemoveFailed)
}
