package certstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func ffPaths(t *testing.T) (policy, sidecar string) {
	d := t.TempDir()
	return filepath.Join(d, "etc", "firefox", "policies", "policies.json"), filepath.Join(d, "data", "firefox-policy.json")
}

func installPaths(t *testing.T, policy string) []string {
	t.Helper()
	var p struct {
		Policies struct {
			Certificates struct{ Install []string }
		} `json:"policies"`
	}
	b, err := os.ReadFile(policy)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &p))
	return p.Policies.Certificates.Install
}

func TestFirefox_CreatesAndDeletesOwnFile(t *testing.T) {
	policy, sidecar := ffPaths(t)
	f := NewFirefox(policy, sidecar)
	require.Equal(t, "firefox", f.Name())
	der := testCA(t, "Ghostline Fake SNI 1")
	require.NoError(t, f.Install(der))
	cert := filepath.Join(filepath.Dir(policy), "ghostline-"+Thumbprint(der)+".crt")
	require.Equal(t, []string{cert}, installPaths(t, policy))
	require.FileExists(t, cert)
	require.NoError(t, f.Install(der), "twice: no duplicate entry")
	require.Len(t, installPaths(t, policy), 1)
	l, err := f.List("Ghostline Fake SNI")
	require.NoError(t, err)
	require.Len(t, l, 1)

	require.NoError(t, f.Remove(Thumbprint(der)))
	require.NoFileExists(t, policy, "Ghostline created it and nothing else is in it")
	require.NoFileExists(t, cert)
	require.NoFileExists(t, sidecar)
}

// Review Focus 3: the admin's policies and certificates stay as they were.
func TestFirefox_KeepsOtherEntries(t *testing.T) {
	policy, sidecar := ffPaths(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(policy), 0o755))
	orig := `{"policies":{"DisableTelemetry":true,"Certificates":{"ImportEnterpriseRoots":true,"Install":["/etc/corp/root.crt"]}}}`
	require.NoError(t, os.WriteFile(policy, []byte(orig), 0o644))
	f := NewFirefox(policy, sidecar)
	der := testCA(t, "Ghostline Fake SNI 2")
	require.NoError(t, f.Install(der))
	require.Equal(t, []string{"/etc/corp/root.crt", filepath.Join(filepath.Dir(policy), "ghostline-"+Thumbprint(der)+".crt")}, installPaths(t, policy))
	require.NoError(t, f.Remove(Thumbprint(der)))
	b, err := os.ReadFile(policy)
	require.NoError(t, err)
	require.JSONEq(t, orig, string(b))
}

// Review Focus 3: a policy file Ghostline cannot parse is never rewritten.
func TestFirefox_MalformedPolicyUntouched(t *testing.T) {
	policy, sidecar := ffPaths(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(policy), 0o755))
	require.NoError(t, os.WriteFile(policy, []byte("{bad"), 0o644))
	der := testCA(t, "Ghostline Fake SNI 3")
	err := NewFirefox(policy, sidecar).Install(der)
	require.Error(t, err)
	b, _ := os.ReadFile(policy)
	require.Equal(t, "{bad", string(b))
	require.NoFileExists(t, filepath.Join(filepath.Dir(policy), "ghostline-"+Thumbprint(der)+".crt"), "no orphan CA file")
}
