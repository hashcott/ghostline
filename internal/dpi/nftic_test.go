package dpi

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type nftFakes struct {
	log      []string
	missing  []string
	probeErr error
}

func (f *nftFakes) ic() *nftInterceptor {
	return &nftInterceptor{
		filter:   testFilter,
		install:  func([]CaptureRule) error { f.log = append(f.log, "install"); return nil },
		remove:   func() error { f.log = append(f.log, "remove"); return nil },
		missing:  func() []string { return f.missing },
		modprobe: func(m string) error { f.log = append(f.log, "modprobe:"+m); f.missing = nil; return f.probeErr },
		queue:    func() ([]byte, error) { return []byte("  200  4242     0 2 65531     0     0        2  1\n"), nil },
		prepDir:  func(dir string) error { f.log = append(f.log, "dir:"+dir); return nil },
	}
}

// Review Focus 1: a restarted daemon prepares over a table that is still
// loaded; installTable replaces it, so Prepare simply runs again.
func TestLinuxInterceptor_PrepareTwice(t *testing.T) {
	f := &nftFakes{}
	ic := f.ic()
	require.NoError(t, ic.Prepare("/e"))
	require.NoError(t, ic.Prepare("/e"))
	require.Equal(t, []string{"dir:/e", "install", "dir:/e", "install"}, f.log)
	require.True(t, ic.Ready(4242))
	require.False(t, ic.Ready(1), "queue 200 is held by another process")
	require.Equal(t, InterceptorInfo{Mechanism: "nftables inet ghostline, queue 200"}, ic.Info())
	require.NoError(t, ic.Cleanup())
	require.Equal(t, "remove", f.log[len(f.log)-1])
}

func TestLinuxInterceptor_LoadsMissingModules(t *testing.T) {
	f := &nftFakes{missing: []string{"nft_queue"}}
	require.NoError(t, f.ic().Prepare("/e"))
	require.Equal(t, []string{"modprobe:nft_queue", "dir:/e", "install"}, f.log)
}

func TestLinuxInterceptor_KernelUnsupported(t *testing.T) {
	f := &nftFakes{missing: []string{"nft_queue"}, probeErr: errors.New("not found")}
	ic := f.ic()
	ic.modprobe = func(string) error { return errors.New("not found") } // still missing afterwards
	err := ic.Prepare("/e")
	require.ErrorIs(t, err, ErrKernelUnsupported)
	require.NotContains(t, f.log, "install")
}
