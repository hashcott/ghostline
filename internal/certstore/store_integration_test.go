//go:build windows && integration

package certstore_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/stretchr/testify/require"
)

// Run by hand once: adding to CurrentUser\Root shows a Windows dialog.
//
//	go test -tags integration ./internal/certstore/ -run Windows
func TestWindows_InstallListRemove(t *testing.T) {
	s := certstore.NewWindows(certstore.CurrentUser)
	ca := session(t)
	require.NoError(t, s.Install(ca.DER))
	t.Cleanup(func() { _ = s.Remove(ca.Thumbprint()) })
	got, err := s.List(certs.SessionPrefix)
	require.NoError(t, err)
	found := false
	for _, c := range got {
		found = found || c.Thumbprint == ca.Thumbprint()
	}
	require.True(t, found)
	require.NoError(t, s.Remove(ca.Thumbprint()))
	require.NoError(t, s.Remove(ca.Thumbprint()))
}

// List never needs elevation and never prompts.
func TestWindows_ListLocalMachine(t *testing.T) {
	_, err := certstore.NewWindows(certstore.LocalMachine).List("Ghostline")
	require.NoError(t, err)
}
