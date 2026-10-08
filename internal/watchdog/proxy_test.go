package watchdog_test

import (
	"errors"
	"github.com/hashcott/ghostline/internal/model"
	"os"
	"testing"

	"github.com/hashcott/ghostline/internal/store"
	"github.com/hashcott/ghostline/internal/sysdns"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/stretchr/testify/require"
)

var proxySnap = model.ProxySnapshot{Backend: "windows", Windows: &model.WinINETProxy{Flags: 1, Bypass: "<local>"}}

func dirtyWithProxy() *store.State {
	st := dirty()
	st.SysProxy = &store.SysProxyState{Set: true, Ours: "127.0.0.1:8080", Snapshot: &proxySnap}
	st.AddFirewallRule("Ghostline Proxy")
	return st
}

// orderDNS records restore calls into a shared log.
type orderDNS struct {
	fakeDNS
	log *[]string
}

func (o *orderDNS) Restore(s sysdns.Snapshot) []sysdns.RestoreError {
	*o.log = append(*o.log, "dns")
	return o.fakeDNS.Restore(s)
}

func (o *orderDNS) RestoreDefault() error {
	*o.log = append(*o.log, "dns")
	return o.fakeDNS.RestoreDefault()
}

func withProxyHooks(d watchdog.Deps, log *[]string, sysErr error) watchdog.Deps {
	d.DNS = &orderDNS{fakeDNS: fakeDNS{loopback: []sysdns.Adapter{{GUID: snap.GUID}}}, log: log}
	d.RestoreSysProxy = func(ours string, s model.ProxySnapshot) (bool, error) {
		*log = append(*log, "sysproxy:"+ours)
		return sysErr == nil, sysErr
	}
	d.DeleteRule = func(name string) error {
		*log = append(*log, "firewall")
		return nil
	}
	return d
}

func TestRestore_ProxyBeforeDNS(t *testing.T) {
	var log []string
	d, _, _ := setup(t, false, dirtyWithProxy())
	d = withProxyHooks(d, &log, nil)
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.Restored, out)
	require.Equal(t, []string{"sysproxy:127.0.0.1:8080", "firewall", "dns"}, log)
	st, _ := d.States.Load()
	require.Equal(t, store.CleanState(), st)
}

func TestRestore_TakenOverSkipsSysProxy(t *testing.T) {
	var log []string
	st := dirtyWithProxy()
	st.SysProxy.TakenOver = true
	d, _, _ := setup(t, false, st)
	d = withProxyHooks(d, &log, nil)
	_, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, []string{"firewall", "dns"}, log)
}

// Final review I7: a crash between Apply and recording Set=true must still
// restore; RestoreIfOurs itself checks the value is still Ghostline's.
func TestRestore_NotSetStillRestoresIfOurs(t *testing.T) {
	var log []string
	st := dirtyWithProxy()
	st.SysProxy.Set = false
	st.Firewall = nil
	d, _, _ := setup(t, false, st)
	d = withProxyHooks(d, &log, nil)
	_, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, []string{"sysproxy:127.0.0.1:8080", "dns"}, log)
}

func TestRestore_SysProxyFailureKeepsState(t *testing.T) {
	var log []string
	d, _, _ := setup(t, false, dirtyWithProxy())
	d = withProxyHooks(d, &log, errors.New("wininet failed"))
	_, err := watchdog.RestoreIfOrphaned(d)
	require.Error(t, err)
	st, _ := d.States.Load()
	require.Equal(t, store.PhaseDNSSet, st.Phase)
	require.NotNil(t, st.SysProxy)
	require.Contains(t, log, "dns") // DNS is still put back
}

func TestRestore_CorruptDeletesFirewallOnly(t *testing.T) {
	var log []string
	d, _, _ := setup(t, false, nil)
	d = withProxyHooks(d, &log, nil)
	require.NoError(t, os.WriteFile(d.States.Path(), []byte("{bad"), 0o644))
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.RestoredFromCorrupt, out)
	require.Equal(t, []string{"firewall", "firewall", "firewall", "firewall", "firewall", "dns"}, log)
}

func TestRestore_OwnerAliveTouchesNothing(t *testing.T) {
	var log []string
	d, _, _ := setup(t, true, dirtyWithProxy())
	d = withProxyHooks(d, &log, nil)
	out, err := watchdog.RestoreIfOrphaned(d)
	require.NoError(t, err)
	require.Equal(t, watchdog.OwnerAlive, out)
	require.Empty(t, log)
}
