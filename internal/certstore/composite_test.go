package certstore

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// failTarget is a Fake store that can fail.
type failTarget struct {
	*Fake
	name string
	err  error
}

func (f failTarget) Name() string { return f.name }
func (f failTarget) Install(der []byte) error {
	if f.err != nil {
		return f.err
	}
	return f.Fake.Install(der)
}

func TestLinuxStore_OptionalFailureIsPartial(t *testing.T) {
	der := testCA(t, "Ghostline Fake SNI 1")
	sys := failTarget{Fake: NewFake(), name: "system"}
	ff := failTarget{Fake: NewFake(), name: "firefox", err: errors.New("read-only")}
	err := NewLinux(sys, ff).Install(der)
	var p *PartialError
	require.ErrorAs(t, err, &p)
	require.Equal(t, []string{"firefox"}, p.Targets)
	require.True(t, sys.Has(Thumbprint(der)))
}

func TestLinuxStore_RequiredFailureFails(t *testing.T) {
	der := testCA(t, "Ghostline Fake SNI 1")
	sys := failTarget{Fake: NewFake(), name: "system", err: errors.New("no anchors dir")}
	ff := failTarget{Fake: NewFake(), name: "firefox"}
	err := NewLinux(sys, ff).Install(der)
	var p *PartialError
	require.False(t, errors.As(err, &p))
	require.Error(t, err)
	require.False(t, ff.Has(Thumbprint(der)), "nothing optional without the system store")
}

func TestLinuxStore_ListIsTheUnionAndRemoveReachesAll(t *testing.T) {
	a, b := testCA(t, "Ghostline Fake SNI A"), testCA(t, "Ghostline Fake SNI B")
	sys, ff := failTarget{Fake: NewFake(), name: "system"}, failTarget{Fake: NewFake(), name: "firefox"}
	require.NoError(t, sys.Install(a))
	require.NoError(t, ff.Install(a))
	require.NoError(t, ff.Install(b)) // left in Firefox only by an earlier run
	s := NewLinux(sys, ff)
	l, err := s.List("Ghostline Fake SNI")
	require.NoError(t, err)
	require.Len(t, l, 2)
	require.NoError(t, s.Remove(Thumbprint(b)))
	require.False(t, ff.Has(Thumbprint(b)))
}
