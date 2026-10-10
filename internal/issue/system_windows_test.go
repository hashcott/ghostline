package issue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystem_ThisWindows(t *testing.T) {
	f := System("0.7.0", false)
	require.Regexp(t, `^Windows 1[01] .*, OS Build \d+\.\d+$`, f.OSVersion)
	require.Equal(t, PkgInstaller, f.Package)
	require.Equal(t, PkgPortable, System("0.7.0", true).Package)
	t.Log(URL("https://github.com/hashcott/ghostline", f))
}
