package certstore

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// firefox adds Ghostline's roots to Firefox's enterprise policy
// (Certificates.Install), which deb, rpm and snap Firefox all read. Only
// Ghostline's entries are touched; a file Ghostline created (recorded in
// sidecar) is deleted again once nothing else is in it.
type firefox struct{ policy, sidecar string }

// NewFirefox edits the policy file at policy; sidecar remembers whether
// Ghostline created it.
func NewFirefox(policy, sidecar string) Target { return firefox{policy: policy, sidecar: sidecar} }

// FirefoxInstalled reports a system Firefox (not Flatpak, which does not
// read /etc).
func FirefoxInstalled() bool {
	for _, p := range []string{"/usr/lib/firefox", "/usr/lib64/firefox", "/usr/lib/firefox-esr", "/snap/firefox"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func (firefox) Name() string { return "firefox" }

func (f firefox) dir() string { return filepath.Dir(f.policy) }

// edit applies fn to policies.Certificates.Install, keeping every other key.
func (f firefox) edit(fn func([]string) []string) error {
	top := map[string]json.RawMessage{}
	b, err := os.ReadFile(f.policy)
	created := false
	switch {
	case errors.Is(err, fs.ErrNotExist):
		created = true
	case err != nil:
		return err
	default:
		if err := json.Unmarshal(b, &top); err != nil {
			return err // never rewrite a file we cannot read
		}
	}
	policies, certsObj := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if raw, ok := top["policies"]; ok {
		if err := json.Unmarshal(raw, &policies); err != nil {
			return err
		}
	}
	if raw, ok := policies["Certificates"]; ok {
		if err := json.Unmarshal(raw, &certsObj); err != nil {
			return err
		}
	}
	var install []string
	if raw, ok := certsObj["Install"]; ok {
		if err := json.Unmarshal(raw, &install); err != nil {
			return err
		}
	}
	install = fn(install)
	set := func(m map[string]json.RawMessage, k string, v any, empty bool) {
		if empty {
			delete(m, k)
			return
		}
		m[k], _ = json.Marshal(v)
	}
	set(certsObj, "Install", install, len(install) == 0)
	set(policies, "Certificates", certsObj, len(certsObj) == 0)
	top["policies"], _ = json.Marshal(policies)

	ours := created || f.ownsFile()
	if ours && len(policies) == 0 && len(top) == 1 {
		// Ghostline's own file with nothing else in it: gone again.
		if err := os.Remove(f.policy); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		_ = os.Remove(f.sidecar)
		return nil
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.dir(), 0o755); err != nil {
		return err
	}
	tmp := f.policy + ".ghostline.tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, f.policy); err != nil {
		return err
	}
	if created {
		if err := os.MkdirAll(filepath.Dir(f.sidecar), 0o700); err != nil {
			return err
		}
		return os.WriteFile(f.sidecar, []byte(`{"created":true}`+"\n"), 0o600)
	}
	return nil
}

func (f firefox) ownsFile() bool { _, err := os.Stat(f.sidecar); return err == nil }

func (f firefox) Install(der []byte) error {
	path, err := writeCert(f.dir(), der)
	if err != nil {
		return err
	}
	err = f.edit(func(in []string) []string {
		if slices.Contains(in, path) {
			return in
		}
		return append(in, path)
	})
	if err != nil {
		_, _ = removeCert(f.dir(), Thumbprint(der))
	}
	return err
}

func (f firefox) Remove(thumbprint string) error {
	path := certFile(f.dir(), thumbprint)
	if err := f.edit(func(in []string) []string {
		return slices.DeleteFunc(in, func(p string) bool { return p == path })
	}); err != nil {
		return err
	}
	_, err := removeCert(f.dir(), thumbprint)
	return err
}

func (f firefox) List(prefix string) ([]Cert, error) { return readCerts(f.dir(), prefix) }
