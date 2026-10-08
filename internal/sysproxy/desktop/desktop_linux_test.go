package desktop

import (
	"encoding/json"
	"testing"

	"github.com/hashcott/ghostline/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBackendFor(t *testing.T) {
	for desktop, want := range map[string]string{
		"KDE": "kde", "GNOME": "gnome", "ubuntu:GNOME": "gnome", "X-Cinnamon": "gnome", "Cinnamon": "gnome",
		"Budgie:GNOME": "gnome", "Unity": "gnome", "XFCE": "", "sway": "", "": "",
	} {
		require.Equal(t, want, backendFor(desktop), desktop)
	}
}

// The agent's tasks work off the session's desktop and HOME, and restore
// through the backend the snapshot names.
func TestAgentTasks_SnapshotThenRestore(t *testing.T) {
	f := &fakeKwrite{keys: map[string]string{"ProxyType": "0"}}
	env := map[string]string{"XDG_CURRENT_DESKTOP": "KDE", "HOME": t.TempDir()}
	tasks := agentTasks(func(k string) string { return env[k] }, func(string, string) (desktop, bool) { return fakeKDE(f), true })
	res, err := tasks["proxy.snapshot"](json.RawMessage(`{"ours":"127.0.0.1:8080"}`))
	require.NoError(t, err)
	snap := res.(model.ProxySnapshot)
	require.Equal(t, "kde", snap.Backend)
	_, err = tasks["proxy.apply"](json.RawMessage(`{"addr":"127.0.0.1:8080"}`))
	require.NoError(t, err)
	ours, err := tasks["proxy.isOurs"](json.RawMessage(`{"addr":"127.0.0.1:8080"}`))
	require.NoError(t, err)
	require.Equal(t, true, ours)
	b, _ := json.Marshal(map[string]any{"snapshot": snap})
	_, err = tasks["proxy.restore"](b)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"ProxyType": "0"}, f.keys)
}

func TestAgentTasks_UnsupportedDesktop(t *testing.T) {
	tasks := agentTasks(func(k string) string { return map[string]string{"XDG_CURRENT_DESKTOP": "XFCE"}[k] }, desktopNamed)
	_, err := tasks["proxy.snapshot"](json.RawMessage(`{}`))
	require.ErrorContains(t, err, "PROXY_DESKTOP_UNSUPPORTED")
}
