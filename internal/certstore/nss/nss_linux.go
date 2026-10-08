// Package nss trusts Fake SNI roots in a user's NSS database
// (~/.pki/nssdb, read by Chrome and Chromium) where NSS does not read the
// system store itself: Debian and Ubuntu. certutil runs in the user's
// session agent.
package nss

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/session"
)

type target struct {
	s      session.Sessions
	q      *session.Queue
	users  string // uid → roots installed there, so they are removed again
	p11kit func() bool
	mu     sync.Mutex
}

// New installs into the graphical session user's NSS database; usersPath
// records which users got which roots.
func New(s session.Sessions, q *session.Queue, usersPath string, p11kit func() bool) certstore.Target {
	return &target{s: s, q: q, users: usersPath, p11kit: p11kit}
}

// P11KitTrust reports NSS reading the system store through p11-kit
// (libnssckbi.so is p11-kit-trust.so): then nothing per user is needed.
func P11KitTrust() bool {
	for _, dir := range []string{"/usr/lib", "/usr/lib64", "/usr/lib/x86_64-linux-gnu"} {
		for _, sub := range []string{"", "nss"} {
			p, err := filepath.EvalSymlinks(filepath.Join(dir, sub, "libnssckbi.so"))
			if err == nil && strings.Contains(filepath.Base(p), "p11-kit") {
				return true
			}
		}
	}
	return false
}

func (*target) Name() string { return "nss" }

type installArgs struct {
	DER []byte `json:"der"`
}

type installResult struct {
	Skipped bool `json:"skipped"` // the user has no NSS database (no Chrome)
}

type removeArgs struct {
	UID        int    `json:"uid,omitempty"`
	Thumbprint string `json:"thumbprint"`
}

func (t *target) load() (map[string][]certstore.Cert, error) {
	m := map[string][]certstore.Cert{}
	b, err := os.ReadFile(t.users)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(b, &m)
}

func (t *target) save(m map[string][]certstore.Cert) error {
	for k, v := range m {
		if len(v) == 0 {
			delete(m, k)
		}
	}
	if len(m) == 0 {
		if err := os.Remove(t.users); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tmp := t.users + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, t.users)
}

func agentErr(err error) error {
	var se *session.Error
	if errors.As(err, &se) && se.Code == "CERT_NSS_TOOL_MISSING" {
		return fmt.Errorf("%w: %v", certstore.ErrNSSToolMissing, err)
	}
	return err
}

func (t *target) Install(der []byte) error {
	if t.p11kit() {
		return nil
	}
	u, ok := t.s.Active()
	if !ok {
		return nil // nobody's browser to reach
	}
	cert, err := certstore.Describe(der)
	if err != nil {
		return err
	}
	// Recorded first: a crash during certutil still leads to a removal.
	if err := t.record(u.UID, cert, true); err != nil {
		return err
	}
	var res installResult
	if err := t.s.Run(u, "nss.install", installArgs{DER: der}, &res); err != nil {
		return agentErr(err)
	}
	if res.Skipped {
		return t.record(u.UID, cert, false) // no database: nothing installed
	}
	return nil
}

// record adds (or drops) cert for uid in the users file.
func (t *target) record(uid int, cert certstore.Cert, add bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, err := t.load()
	if err != nil {
		return err
	}
	key := strconv.Itoa(uid)
	keep := m[key][:0]
	for _, c := range m[key] {
		if c.Thumbprint != cert.Thumbprint {
			keep = append(keep, c)
		}
	}
	if add {
		keep = append(keep, cert)
	}
	m[key] = keep
	return t.save(m)
}

// Remove takes the root out of every user it went to; a user who is not
// logged in gets it removed through the queue.
func (t *target) Remove(thumbprint string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, err := t.load()
	if err != nil {
		return err
	}
	var errs []error
	for key, certs := range m {
		uid, _ := strconv.Atoi(key)
		var keep []certstore.Cert
		for _, c := range certs {
			if c.Thumbprint != thumbprint {
				keep = append(keep, c)
				continue
			}
			if u, ok := t.s.ByUID(uid); ok {
				if err := t.s.Run(u, "nss.remove", removeArgs{Thumbprint: thumbprint}, nil); err != nil {
					errs = append(errs, agentErr(err))
					keep = append(keep, c)
				}
			} else if err := t.q.Add(uid, "nss.remove", removeArgs{UID: uid, Thumbprint: thumbprint}); err != nil {
				errs = append(errs, err)
				keep = append(keep, c)
			}
		}
		m[key] = keep
	}
	return errors.Join(append(errs, t.save(m))...)
}

func (t *target) List(prefix string) ([]certstore.Cert, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	m, err := t.load()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []certstore.Cert
	for _, certs := range m {
		for _, c := range certs {
			if strings.HasPrefix(c.Subject, prefix) && !seen[c.Thumbprint] {
				seen[c.Thumbprint] = true
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// QueueHandler runs queued NSS removals.
func QueueHandler(s session.Sessions) func(task string, args json.RawMessage) error {
	return func(task string, args json.RawMessage) error {
		if task != "nss.remove" {
			return fmt.Errorf("nss: not an NSS task: %s", task)
		}
		var r removeArgs
		if err := json.Unmarshal(args, &r); err != nil {
			return err
		}
		u, ok := s.ByUID(r.UID)
		if !ok {
			return errors.New("nss: user not logged in")
		}
		return agentErr(s.Run(u, "nss.remove", removeArgs{Thumbprint: r.Thumbprint}, nil))
	}
}
