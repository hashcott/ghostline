package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTerms_AcceptRecordsVersionAndTime(t *testing.T) {
	h := newSvc(t)
	require.False(t, h.svc.TermsAccepted(), "a new install has not accepted the terms")
	require.NoError(t, h.svc.AcceptTerms())
	got := h.box.Get().Terms
	require.Equal(t, TermsVersion, got.AckVersion)
	at, err := time.Parse(time.RFC3339, got.AckAt)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now(), at, time.Minute)
	require.True(t, h.svc.TermsAccepted())
}

func TestTerms_SaveSettingsCannotChangeThem(t *testing.T) {
	h := newSvc(t)
	// The window's copy says accepted: only AcceptTerms may record that.
	n := h.box.Get()
	n.Terms.AckVersion = TermsVersion
	require.NoError(t, h.svc.SaveSettings(n))
	require.False(t, h.svc.TermsAccepted())

	// A stale copy from before accepting does not undo it.
	stale := h.box.Get()
	require.NoError(t, h.svc.AcceptTerms())
	require.NoError(t, h.svc.SaveSettings(stale))
	require.True(t, h.svc.TermsAccepted())
}
