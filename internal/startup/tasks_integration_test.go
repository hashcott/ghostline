//go:build windows && integration

// Run from an elevated terminal: go test -tags integration ./internal/startup/...
package startup_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/startup"
	"github.com/stretchr/testify/require"
)

func TestIntegration_CreateExistsDelete(t *testing.T) {
	task := startup.RecoveryTask(`C:\Windows\System32\cmd.exe`)
	task.Name = "Ghostline Test"
	require.NoError(t, startup.Create(task))
	t.Cleanup(func() { _ = startup.Delete(task.Name) })
	require.True(t, startup.Exists(task.Name))
	require.NoError(t, startup.Delete(task.Name))
	require.False(t, startup.Exists(task.Name))
	require.NoError(t, startup.Delete(task.Name), "deleting a missing task is not an error")
}

func TestIntegration_QueryXMLMatchesWhatCreateWrote(t *testing.T) {
	task := startup.AutostartTask(`C:\Program Files\Ghostline Test\ghostline.exe`)
	task.Name = "Ghostline Test Autostart"
	require.NoError(t, startup.Create(task))
	t.Cleanup(func() { _ = startup.Delete(task.Name) })
	x, err := startup.QueryXML(task.Name)
	require.NoError(t, err)
	require.True(t, startup.TaskMatches(x, task), x)
	other := task
	other.Exe = `C:\Other\ghostline.exe`
	require.False(t, startup.TaskMatches(x, other))
	_, err = startup.QueryXML("Ghostline Test Missing")
	require.Error(t, err)
}
