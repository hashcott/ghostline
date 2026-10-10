package issue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystem_ThisWindows(t *testing.T) {
	f := System("0.7.0", false)
	require.Contains(t, []string{"Windows 10", "Windows 11"}, f.OS)
	require.Contains(t, f.OSVersion, ", OS Build ")
	require.Equal(t, PkgInstaller, f.Package)
	require.Equal(t, PkgPortable, System("0.7.0", true).Package)
	t.Log(URL("https://github.com/hashcott/ghostline", f))
}
