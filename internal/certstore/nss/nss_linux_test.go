package nss

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/session"
	"github.com/stretchr/testify/require"
)

func testCA(t *testing.T, cn string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	n, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{SerialNumber: n, Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return der
}

type call struct {
	uid  int
	task string
}

type fakeSessions struct {
	active  *session.User
	present map[int]session.User
	calls   []call
	err     error
	answer  any
}

func (f *fakeSessions) Active() (session.User, bool) {
	if f.active == nil {
		return session.User{}, false
	}
	return *f.active, true
}
func (f *fakeSessions) ByUID(uid int) (session.User, bool) { u, ok := f.present[uid]; return u, ok }
func (f *fakeSessions) Run(u session.User, task string, in, out any) error {
	f.calls = append(f.calls, call{u.UID, task})
	if f.err != nil {
		return f.err
	}
	if out != nil && f.answer != nil {
		b, _ := json.Marshal(f.answer)
		return json.Unmarshal(b, out)
	}
	return nil
}
func (f *fakeSessions) Stream(session.User, string, any, func(json.RawMessage)) (func(), error) {
	return func() {}, nil
}
func (f *fakeSessions) WatchNew(func(session.User)) (func(), error) { return func() {}, nil }

var me = session.User{UID: 1000, Name: "harry", Home: "/home/harry"}

func newT(t *testing.T, f *fakeSessions, p11 bool) (certstore.Target, string) {
	dir := t.TempDir()
	return New(f, session.NewQueue(filepath.Join(dir, "q.json")), filepath.Join(dir, "nss-users.json"), func() bool { return p11 }), dir
}

// Fedora, Arch: NSS reads the system store through p11-kit already.
func TestNSS_SkippedWithP11Kit(t *testing.T) {
	f := &fakeSessions{active: &me}
	n, _ := newT(t, f, true)
	require.NoError(t, n.Install(testCA(t, "Ghostline Fake SNI 1")))
	require.Empty(t, f.calls)
}

func TestNSS_InstallRecordsUIDAndRemoves(t *testing.T) {
	f := &fakeSessions{active: &me, present: map[int]session.User{1000: me}, answer: installResult{}}
	n, _ := newT(t, f, false)
	der := testCA(t, "Ghostline Fake SNI 1")
	require.NoError(t, n.Install(der))
	l, err := n.List("Ghostline Fake SNI")
	require.NoError(t, err)
	require.Len(t, l, 1)
	require.Equal(t, certstore.Thumbprint(der), l[0].Thumbprint)
	require.NoError(t, n.Remove(certstore.Thumbprint(der)))
	require.Equal(t, []call{{1000, "nss.install"}, {1000, "nss.remove"}}, f.calls)
	l, _ = n.List("Ghostline Fake SNI")
	require.Empty(t, l)
}

// No ~/.pki/nssdb (no Chrome): nothing installed, nothing to remember.
func TestNSS_NoDatabaseNothingRecorded(t *testing.T) {
	f := &fakeSessions{active: &me, answer: installResult{Skipped: true}}
	n, _ := newT(t, f, false)
	require.NoError(t, n.Install(testCA(t, "Ghostline Fake SNI 1")))
	l, _ := n.List("")
	require.Empty(t, l)
}

func TestNSS_RemoveQueuesWhenAway(t *testing.T) {
	f := &fakeSessions{active: &me, present: map[int]session.User{1000: me}, answer: installResult{}}
	dir := t.TempDir()
	q := session.NewQueue(filepath.Join(dir, "q.json"))
	n := New(f, q, filepath.Join(dir, "nss-users.json"), func() bool { return false })
	der := testCA(t, "Ghostline Fake SNI 1")
	require.NoError(t, n.Install(der))
	f.present = nil // logged out
	require.NoError(t, n.Remove(certstore.Thumbprint(der)))
	require.Len(t, f.calls, 1, "no agent to run now")

	f.present = map[int]session.User{1000: me}
	require.NoError(t, q.Drain(me, QueueHandler(f)))
	require.Equal(t, call{1000, "nss.remove"}, f.calls[1])
}

func TestNSS_ToolMissingIsKnownError(t *testing.T) {
	f := &fakeSessions{active: &me, err: &session.Error{Code: "CERT_NSS_TOOL_MISSING"}}
	n, _ := newT(t, f, false)
	err := n.Install(testCA(t, "Ghostline Fake SNI 1"))
	require.ErrorIs(t, err, certstore.ErrNSSToolMissing)
}

// fakeCertutil records argv.
type fakeCertutil struct{ argv [][]string }

func (f *fakeCertutil) run(name string, args ...string) ([]byte, error) {
	f.argv = append(f.argv, append([]string{name}, args...))
	return nil, nil
}

func TestNSSAgent_CertutilArgs(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".pki", "nssdb"), 0o700))
	f := &fakeCertutil{}
	tasks := agentTasks(home, f.run, func(string) (string, error) { return "/usr/bin/certutil", nil })
	der := testCA(t, "Ghostline Fake SNI 9")
	b, _ := json.Marshal(installArgs{DER: der})
	res, err := tasks["nss.install"](b)
	require.NoError(t, err)
	require.Equal(t, installResult{}, res)
	db := "sql:" + filepath.Join(home, ".pki", "nssdb")
	nick := "ghostline-" + certstore.Thumbprint(der)
	require.Equal(t, []string{"/usr/bin/certutil", "-d", db, "-A", "-t", "C,,", "-n", nick, "-i"}, f.argv[0][:9])
	b, _ = json.Marshal(removeArgs{Thumbprint: certstore.Thumbprint(der)})
	_, err = tasks["nss.remove"](b)
	require.NoError(t, err)
	require.Equal(t, []string{"/usr/bin/certutil", "-d", db, "-D", "-n", nick}, f.argv[1])
}

func TestNSSAgent_MissingTool(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".pki", "nssdb"), 0o700))
	tasks := agentTasks(home, (&fakeCertutil{}).run, func(string) (string, error) { return "", errors.New("not found") })
	b, _ := json.Marshal(installArgs{DER: testCA(t, "x")})
	_, err := tasks["nss.install"](b)
	var se *session.Error
	require.ErrorAs(t, err, &se)
	require.Equal(t, "CERT_NSS_TOOL_MISSING", se.Code)
}

func TestNSSAgent_NoDatabaseSkips(t *testing.T) {
	tasks := agentTasks(t.TempDir(), (&fakeCertutil{}).run, func(string) (string, error) { return "/usr/bin/certutil", nil })
	b, _ := json.Marshal(installArgs{DER: testCA(t, "x")})
	res, err := tasks["nss.install"](b)
	require.NoError(t, err)
	require.Equal(t, installResult{Skipped: true}, res)
}

// Review I4: the user deleted the CA in Chrome; removing it again is done,
// not an error that keeps the entry (and a queued task) forever.
func TestNSSAgent_RemoveOfMissingCertIsDone(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".pki", "nssdb"), 0o700))
	run := func(string, ...string) ([]byte, error) {
		return []byte("certutil: could not find certificate named \"ghostline-x\": SEC_ERROR_BAD_DATABASE"), errors.New("exit status 255")
	}
	tasks := agentTasks(home, run, func(string) (string, error) { return "/usr/bin/certutil", nil })
	b, _ := json.Marshal(removeArgs{Thumbprint: "x"})
	_, err := tasks["nss.remove"](b)
	require.NoError(t, err)
}

// Review I4: recorded before certutil runs, so a crash in between still
// leads to a removal later.
func TestNSS_RecordsBeforeInstalling(t *testing.T) {
	f := &fakeSessions{active: &me, err: errors.New("agent crashed")}
	n, _ := newT(t, f, false)
	der := testCA(t, "Ghostline Fake SNI 1")
	require.Error(t, n.Install(der))
	l, err := n.List("Ghostline Fake SNI")
	require.NoError(t, err)
	require.Len(t, l, 1, "the attempt is on record")
}

// fakeNSSDBs is certutil over in-memory databases: dir → nickname → DER.
type fakeNSSDBs map[string]map[string][]byte

func (f fakeNSSDBs) run(_ string, args ...string) ([]byte, error) {
	arg := func(flag string) string {
		for i, a := range args {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	db := f[strings.TrimPrefix(arg("-d"), "sql:")]
	nick := arg("-n")
	switch {
	case slices.Contains(args, "-D"):
		if _, ok := db[nick]; !ok {
			return []byte("certutil: could not find certificate named " + nick), errors.New("exit status 255")
		}
		delete(db, nick)
		return nil, nil
	case slices.Contains(args, "-L") && nick != "":
		return db[nick], nil // -r: the DER
	case slices.Contains(args, "-L"):
		out := "\nCertificate Nickname                                         Trust Attributes\n                                                             SSL,S/MIME,JAR/XPI\n\n"
		for n := range db {
			out += fmt.Sprintf("%-60s %s\n", n, "C,,")
		}
		return []byte(out), nil
	}
	return nil, nil
}

func profile(t *testing.T, home string, rel ...string) string {
	t.Helper()
	dir := filepath.Join(append([]string{home}, rel...)...)
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cert9.db"), nil, 0o600))
	return dir
}

// Ubuntu/Debian: Firefox imports the policy's root into each profile and
// keeps it after the policy is gone. Removal takes it out of every profile
// (deb, snap, XDG) by thumbprint; other roots stay.
func TestNSSAgent_RemovePurgesFirefoxProfiles(t *testing.T) {
	home := t.TempDir()
	deb := profile(t, home, ".mozilla", "firefox", "abc.default-release")
	snap := profile(t, home, "snap", "firefox", "common", ".mozilla", "firefox", "x.default")
	xdg := profile(t, home, ".config", "mozilla", "firefox", "y.default")
	der, other := testCA(t, "Ghostline Fake SNI A"), testCA(t, "Ghostline Fake SNI B")
	dbs := fakeNSSDBs{
		deb:  {"Ghostline Fake SNI A - Ghostline": der, "Ghostline Fake SNI B - Ghostline": other, "DigiCert Root": other},
		snap: {"Ghostline Fake SNI A - Ghostline": der},
		xdg:  {},
	}
	tasks := agentTasks(home, dbs.run, func(string) (string, error) { return "/usr/bin/certutil", nil })
	b, _ := json.Marshal(removeArgs{Thumbprint: certstore.Thumbprint(der)})
	_, err := tasks["nss.remove"](b)
	require.NoError(t, err)
	require.Equal(t, map[string][]byte{"Ghostline Fake SNI B - Ghostline": other, "DigiCert Root": other}, dbs[deb])
	require.Empty(t, dbs[snap])
}

// A user with Firefox but no Chrome database stays on record: the policy
// root lands in their Firefox profile and must be purged later.
func TestNSSAgent_FirefoxProfileAloneIsNotSkipped(t *testing.T) {
	home := t.TempDir()
	profile(t, home, ".mozilla", "firefox", "abc.default-release")
	f := &fakeCertutil{}
	tasks := agentTasks(home, f.run, func(string) (string, error) { return "/usr/bin/certutil", nil })
	b, _ := json.Marshal(installArgs{DER: testCA(t, "x")})
	res, err := tasks["nss.install"](b)
	require.NoError(t, err)
	require.Equal(t, installResult{}, res)
	require.Empty(t, f.argv, "the policy installs into Firefox; certutil only for ~/.pki/nssdb")
}
