package firewall

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeUfw struct {
	added []string // `ufw show added` lines
	argv  []string
}

func (f *fakeUfw) run(name string, args ...string) ([]byte, error) {
	f.argv = append(f.argv, name+" "+strings.Join(args, " "))
	if strings.Join(args, " ") == "show added" {
		return []byte("Added user rules (see 'ufw status' for running firewall):\n" + strings.Join(f.added, "\n") + "\n"), nil
	}
	return nil, nil
}

func TestUfw_AddDelete(t *testing.T) {
	f := &fakeUfw{}
	m := newLinux(filepath.Join(t.TempDir(), "f.json"), ufw{run: f.run})
	require.NoError(t, m.Add(8080))
	require.NoError(t, m.Delete())
	require.Equal(t, []string{
		"ufw show added",
		"ufw allow 8080/tcp comment ghostline: Ghostline Proxy",
		"ufw delete allow 8080/tcp",
	}, f.argv)
}

// Review Focus 4: the user's own `ufw allow 8080/tcp` is neither doubled
// nor deleted.
func TestUfw_PreexistingRuleNotRemoved(t *testing.T) {
	f := &fakeUfw{added: []string{"ufw allow 22/tcp", "ufw allow 8080/tcp comment 'my proxy'"}}
	m := newLinux(filepath.Join(t.TempDir(), "f.json"), ufw{run: f.run})
	require.NoError(t, m.Add(8080))
	require.NoError(t, m.Delete())
	require.Equal(t, []string{"ufw show added"}, f.argv)
}

func TestUfw_ExistsMatchesWholePort(t *testing.T) {
	f := &fakeUfw{added: []string{"ufw allow 80800/tcp", "ufw allow 8080/udp"}}
	ok, err := ufw{run: f.run}.exists("", "tcp", 8080)
	require.NoError(t, err)
	require.False(t, ok)
}
