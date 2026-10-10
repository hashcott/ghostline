package shell

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstallerRunner_OnlyInstalledBuilds(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "ghostline.exe")
	require.Nil(t, installerRunner(exe, false, func() {}), "no uninstall.exe: not installed by the installer")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uninstall.exe"), nil, 0o644))
	require.NotNil(t, installerRunner(exe, false, func() {}))
	require.Nil(t, installerRunner(exe, true, func() {}), "portable")
}
