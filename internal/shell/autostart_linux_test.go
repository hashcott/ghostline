package shell

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/stretchr/testify/require"
)

func TestSyncAutostart_WritesAndRemoves(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "autostart")
	exe := "/opt/My Apps/Ghostline.AppImage"
	require.NoError(t, syncAutostart(dir, exe, true))
	b, err := os.ReadFile(filepath.Join(dir, "ghostline.desktop"))
	require.NoError(t, err)
	got := string(b)
	// Review Focus 4: a path with spaces is quoted.
	require.Contains(t, got, "Exec=\"/opt/My Apps/Ghostline.AppImage\" --autostart\n")
	for _, line := range []string{"[Desktop Entry]\n", "Type=Application\n", "Name=Ghostline\n", "Icon=ghostline\n", "X-GNOME-Autostart-enabled=true\n"} {
		require.Contains(t, got, line)
	}
	require.NoError(t, syncAutostart(dir, exe, false))
	_, err = os.Stat(filepath.Join(dir, "ghostline.desktop"))
	require.True(t, os.IsNotExist(err))
	require.NoError(t, syncAutostart(dir, exe, false), "already off")
}

func TestSyncAutostart_LeavesUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syncAutostart(dir, "/usr/bin/ghostline", true))
	p := filepath.Join(dir, "ghostline.desktop")
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(p, old, old))
	require.NoError(t, syncAutostart(dir, "/usr/bin/ghostline", true))
	fi, err := os.Stat(p)
	require.NoError(t, err)
	require.True(t, fi.ModTime().Equal(old), "not rewritten")
}

func TestDesktopExecQuote(t *testing.T) {
	require.Equal(t, `"/usr/bin/ghostline"`, desktopQuote("/usr/bin/ghostline"))
	require.Equal(t, `"/a \"b\" \$c \`+"`"+`d\`+"`"+` \\e"`, desktopQuote(`/a "b" $c `+"`"+`d`+"`"+` \e`))
}

func TestAutostartExe_PrefersAppImage(t *testing.T) {
	env := map[string]string{"APPIMAGE": "/home/u/Ghostline.AppImage"}
	exe := func() (string, error) { return "/tmp/.mount_x/usr/bin/ghostline", nil }
	got, err := autostartExe(func(k string) string { return env[k] }, exe)
	require.NoError(t, err)
	require.Equal(t, "/home/u/Ghostline.AppImage", got)
	got, err = autostartExe(func(string) string { return "" }, exe)
	require.NoError(t, err)
	require.Equal(t, "/tmp/.mount_x/usr/bin/ghostline", got)
	_, err = autostartExe(func(string) string { return "" }, func() (string, error) { return "", errors.New("no exe") })
	require.Error(t, err)
}

func TestAutostartDir(t *testing.T) {
	require.Equal(t, "/x/autostart", autostartDir(func(k string) string { return map[string]string{"XDG_CONFIG_HOME": "/x", "HOME": "/h"}[k] }))
	require.Equal(t, "/h/.config/autostart", autostartDir(func(k string) string { return map[string]string{"HOME": "/h"}[k] }))
}

// Settings from an unreachable daemon are empty: they must not remove the
// user's autostart entry.
func TestApplyAutostart_SkipsSettingsNotFromDaemon(t *testing.T) {
	dir := t.TempDir()
	env := func(k string) string { return map[string]string{"XDG_CONFIG_HOME": dir}[k] }
	exe := func() (string, error) { return "/usr/bin/ghostline", nil }
	on := store.DefaultSettings()
	on.StartWithWindows = true
	require.NoError(t, applyAutostart(on, env, exe))
	_, err := os.Stat(filepath.Join(dir, "autostart", "ghostline.desktop"))
	require.NoError(t, err)
	require.NoError(t, applyAutostart(store.Settings{}, env, exe))
	_, err = os.Stat(filepath.Join(dir, "autostart", "ghostline.desktop"))
	require.NoError(t, err, "empty settings leave it alone")
	off := store.DefaultSettings()
	require.NoError(t, applyAutostart(off, env, exe))
	_, err = os.Stat(filepath.Join(dir, "autostart", "ghostline.desktop"))
	require.True(t, os.IsNotExist(err))
}
