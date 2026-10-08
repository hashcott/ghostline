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
				return installResult{Skipped: true}, nil // no Chrome profile
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
			if _, err := os.Stat(dbDir); err != nil {
				return nil, nil
			}
			cu, err := certutil()
			if err != nil {
				return nil, err
			}
			_, err = run(cu, "-d", db, "-D", "-n", nickname(a.Thumbprint))
			return nil, err
		},
	}
}
