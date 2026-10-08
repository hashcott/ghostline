package firewall

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirewallArgs(t *testing.T) {
	exe := `C:\Program Files\Ghostline Đức\ghostline.exe`
	require.Equal(t, []string{"advfirewall", "firewall", "add", "rule", "name=Ghostline Proxy", "dir=in", "action=allow",
		"protocol=TCP", "localport=8080", "program=" + exe, "profile=private", "remoteip=localsubnet"}, AddArgs(8080, exe))
	require.Equal(t, []string{"advfirewall", "firewall", "delete", "rule", "name=Ghostline Proxy"}, DeleteArgs())
}

type fakeNetsh struct {
	calls [][]string
	fail  map[string]bool // by verb: add, delete, show
}

func (f *fakeNetsh) run(args []string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.fail[args[2]] {
		return []byte("localized failure text"), errors.New("exit status 1")
	}
	return []byte("Ok."), nil
}

func netshWith(f *fakeNetsh) *Netsh { return &Netsh{Exe: `C:\g.exe`, Run: f.run} }

func TestDelete_NoMatchIsNil(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"delete": true, "show": true}}
	require.NoError(t, netshWith(f).Delete())
	require.Equal(t, "show", f.calls[1][2])
}

func TestDelete_RealFailure(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"delete": true}} // rule exists but cannot be deleted
	require.Error(t, netshWith(f).Delete())
}

func TestAdd_ReplacesExisting(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"show": true}}
	require.NoError(t, netshWith(f).Add(8080))
	require.Equal(t, "delete", f.calls[0][2])
	require.Equal(t, "add", f.calls[len(f.calls)-1][2])

	f = &fakeNetsh{fail: map[string]bool{"add": true}}
	require.Error(t, netshWith(f).Add(8080))
}

func TestParseCategories(t *testing.T) {
	require.True(t, parsePublic("Private\r\nPublic\r\n"))
	require.False(t, parsePublic("Private\r\nDomainAuthenticated\r\n"))
	require.False(t, parsePublic(""))
}

func TestRuleArgs(t *testing.T) {
	exe := `C:\g.exe`
	tail := []string{"program=" + exe, "profile=private", "remoteip=localsubnet"}
	want := func(name, proto, ports string) []string {
		return append([]string{"advfirewall", "firewall", "add", "rule", "name=" + name, "dir=in", "action=allow",
			"protocol=" + proto, "localport=" + ports}, tail...)
	}
	require.Equal(t, want("Ghostline DNS (TCP)", "TCP", "53,443"), RuleArgs(Rule{Name: RuleDNSTCP, Protocol: "TCP", Ports: []int{53, 443}}, exe))
	require.Equal(t, want("Ghostline DNS (UDP)", "UDP", "53"), RuleArgs(Rule{Name: RuleDNSUDP, Protocol: "UDP", Ports: []int{53}}, exe))
	require.Equal(t, want("Ghostline Setup", "TCP", "8053"), RuleArgs(Rule{Name: RuleSetup, Protocol: "TCP", Ports: []int{8053}}, exe))
}

func TestNamedRule_AddDelete(t *testing.T) {
	f := &fakeNetsh{fail: map[string]bool{"show": true}}
	require.NoError(t, netshWith(f).AddNamed(Rule{Name: RuleDNSUDP, Protocol: "UDP", Ports: []int{53}}))
	require.Equal(t, []string{"advfirewall", "firewall", "delete", "rule", "name=Ghostline DNS (UDP)"}, f.calls[0])
	require.Equal(t, "add", f.calls[len(f.calls)-1][2])

	f = &fakeNetsh{fail: map[string]bool{"delete": true, "show": true}}
	require.NoError(t, netshWith(f).DeleteNamed(RuleSetup))
	require.Equal(t, []string{"advfirewall", "firewall", "show", "rule", "name=Ghostline Setup"}, f.calls[1])
}

func TestRuleArgs_BlockPublic(t *testing.T) {
	exe := `C:\g.exe`
	require.Equal(t, []string{"advfirewall", "firewall", "add", "rule", "name=Ghostline Block Public", "dir=in", "action=block",
		"program=" + exe, "profile=public"}, RuleArgs(BlockPublicRule, exe))
	require.Contains(t, AllRuleNames, RuleBlockPublic)
}

func TestIsPublicNetwork_UsesProbe(t *testing.T) {
	n := &Netsh{Public: func() (bool, error) { return true, nil }}
	pub, err := n.IsPublicNetwork()
	require.NoError(t, err)
	require.True(t, pub)
}

func TestRuleNames_Unchanged(t *testing.T) {
	// v0.5 state.json files name these rules; renaming one strands it on users' machines.
	require.Equal(t, []string{"Ghostline Proxy", "Ghostline DNS (TCP)", "Ghostline DNS (UDP)", "Ghostline Setup", "Ghostline Block Public"}, AllRuleNames)
}

func TestUnsupported_RemovesAreNoOps(t *testing.T) {
	var m Manager = Unsupported{}
	require.NoError(t, m.Delete())
	require.NoError(t, m.DeleteNamed(RuleSetup))
	require.ErrorIs(t, m.Add(8080), errors.ErrUnsupported)
	require.ErrorIs(t, m.AddNamed(BlockPublicRule), errors.ErrUnsupported)
	_, err := m.IsPublicNetwork()
	require.ErrorIs(t, err, errors.ErrUnsupported)
}
