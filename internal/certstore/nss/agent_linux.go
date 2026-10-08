package nss

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/session"
)

// AgentTasks are the NSS tasks the session agent serves.
func AgentTasks() map[string]session.Task {
	run := func(name string, args ...string) ([]byte, error) {
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			return out, errors.New(err.Error() + ": " + strings.TrimSpace(string(out)))
		}
		return out, nil
	}
	return agentTasks(os.Getenv("HOME"), run, exec.LookPath)
}

func nickname(thumbprint string) string { return "ghostline-" + thumbprint }

func agentTasks(home string, run func(string, ...string) ([]byte, error), look func(string) (string, error)) map[string]session.Task {
	dbDir := filepath.Join(home, ".pki", "nssdb")
	db := "sql:" + dbDir
	certutil := func() (string, error) {
		p, err := look("certutil")
		if err != nil {
			return "", &session.Error{Code: "CERT_NSS_TOOL_MISSING", Message: "certutil not found"}
		}
		return p, nil
	}
	return map[string]session.Task{
		"nss.install": func(raw json.RawMessage) (any, error) {
			var a installArgs
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, err
			}
			if _, err := os.Stat(dbDir); err != nil {
				// No Chrome database. Firefox gets the root from its policy
				// and keeps it in its profiles: the removal purges them.
				return installResult{Skipped: len(firefoxProfiles(home)) == 0}, nil
			}
			cu, err := certutil()
			if err != nil {
				return nil, err
			}
			f, err := os.CreateTemp("", "ghostline-*.der")
			if err != nil {
				return nil, err
			}
			defer os.Remove(f.Name())
			if _, err := f.Write(a.DER); err != nil {
				f.Close()
				return nil, err
			}
			f.Close()
			_, err = run(cu, "-d", db, "-A", "-t", "C,,", "-n", nickname(certstore.Thumbprint(a.DER)), "-i", f.Name())
			return installResult{}, err
		},
		"nss.remove": func(raw json.RawMessage) (any, error) {
			var a removeArgs
			if err := json.Unmarshal(raw, &a); err != nil {
				return nil, err
			}
			_, noDB := os.Stat(dbDir)
			profiles := firefoxProfiles(home)
			if noDB != nil && len(profiles) == 0 {
				return nil, nil
			}
			cu, err := certutil()
			if err != nil {
				return nil, err
			}
			var errs []error
			if noDB == nil {
				out, err := run(cu, "-d", db, "-D", "-n", nickname(a.Thumbprint))
				if err != nil && !strings.Contains(string(out)+err.Error(), "could not find certificate") {
					errs = append(errs, err) // not found: deleted in the browser, done
				}
			}
			for _, dir := range profiles {
				errs = append(errs, purgeProfile(run, cu, dir, a.Thumbprint))
			}
			return nil, errors.Join(errs...)
		},
	}
}

// firefoxProfiles lists the Firefox profiles (deb, snap, XDG) of home.
func firefoxProfiles(home string) []string {
	var out []string
	for _, base := range []string{".mozilla/firefox", "snap/firefox/common/.mozilla/firefox", ".config/mozilla/firefox"} {
		dbs, _ := filepath.Glob(filepath.Join(home, base, "*", "cert9.db"))
		for _, db := range dbs {
			out = append(out, filepath.Dir(db))
		}
	}
	return out
}

// purgeProfile deletes the root with thumbprint from a Firefox profile.
// Firefox names a policy root after its subject ("Ghostline Fake SNI … -
// Ghostline"): those are read back and compared, so nothing else goes.
func purgeProfile(run func(string, ...string) ([]byte, error), cu, dir, thumbprint string) error {
	db := "sql:" + dir
	out, err := run(cu, "-d", db, "-L")
	if err != nil {
		return err
	}
	for _, nick := range listedNicknames(out) {
		if !strings.HasPrefix(nick, "Ghostline") {
			continue
		}
		der, err := run(cu, "-d", db, "-L", "-n", nick, "-r")
		if err != nil || certstore.Thumbprint(der) != thumbprint {
			continue
		}
		if _, err := run(cu, "-d", db, "-D", "-n", nick); err != nil {
			return err
		}
	}
	return nil
}

// listedNicknames reads `certutil -L`: a header, then one line per
// certificate, its nickname and then its trust flags.
func listedNicknames(out []byte) []string {
	var names []string
	header := true
	for line := range strings.Lines(string(out)) {
		if header {
			header = !strings.Contains(line, "SSL,S/MIME,JAR/XPI")
			continue
		}
		line = strings.TrimRight(line, " \t\r\n")
		if i := strings.LastIndexAny(line, " \t"); i > 0 {
			names = append(names, strings.TrimSpace(line[:i]))
		}
	}
	return names
}
