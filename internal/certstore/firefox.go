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

// sidecarFile records that Ghostline created the policy file, and which
// directories it created for it.
type sidecarFile struct {
	Created bool     `json:"created"`
	Dirs    []string `json:"dirs,omitempty"`
}

// missingDirs lists dir and its ancestors that do not exist, deepest first.
func missingDirs(dir string) []string {
	var out []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || d == filepath.Dir(d) {
			return out
		}
		out = append(out, d)
	}
}

// edit applies fn to policies.Certificates.Install, keeping every other key.
// It reports whether it deleted Ghostline's own file, and the directories
// recorded as Ghostline's (to remove once the CA files are gone).
// newDirs are directories this install created.
func (f firefox) edit(fn func([]string) []string, newDirs []string) (deleted bool, dirs []string, err error) {
	top := map[string]json.RawMessage{}
	b, err := os.ReadFile(f.policy)
	created := false
	switch {
	case errors.Is(err, fs.ErrNotExist):
		created = true
	case err != nil:
		return false, nil, err
	default:
		if err := json.Unmarshal(b, &top); err != nil {
			return false, nil, err // never rewrite a file we cannot read
		}
	}
	policies, certsObj := map[string]json.RawMessage{}, map[string]json.RawMessage{}
	if raw, ok := top["policies"]; ok {
		if err := json.Unmarshal(raw, &policies); err != nil {
			return false, nil, err
		}
	}
	if raw, ok := policies["Certificates"]; ok {
		if err := json.Unmarshal(raw, &certsObj); err != nil {
			return false, nil, err
		}
	}
	var install []string
	if raw, ok := certsObj["Install"]; ok {
		if err := json.Unmarshal(raw, &install); err != nil {
			return false, nil, err
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

	side, owns := f.readSidecar()
	if (created || owns) && len(policies) == 0 && len(top) == 1 {
		// Ghostline's own file with nothing else in it: gone again.
		if err := os.Remove(f.policy); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return false, nil, err
		}
		_ = os.Remove(f.sidecar)
		return true, side.Dirs, nil
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return false, nil, err
	}
	if err := os.MkdirAll(f.dir(), 0o755); err != nil {
		return false, nil, err
	}
	tmp := f.policy + ".ghostline.tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return false, nil, err
	}
	if err := os.Rename(tmp, f.policy); err != nil {
		return false, nil, err
	}
	if created {
		if err := os.MkdirAll(filepath.Dir(f.sidecar), 0o700); err != nil {
			return false, nil, err
		}
		b, _ := json.Marshal(sidecarFile{Created: true, Dirs: newDirs})
		return false, nil, os.WriteFile(f.sidecar, append(b, '\n'), 0o600)
	}
	return false, nil, nil
}

// readSidecar reports whether Ghostline created the policy file.
func (f firefox) readSidecar() (sidecarFile, bool) {
	b, err := os.ReadFile(f.sidecar)
	if err != nil {
		return sidecarFile{}, false
	}
	var side sidecarFile
	if json.Unmarshal(b, &side) != nil {
		return sidecarFile{Created: true}, true // the v1 form: just a marker
	}
	return side, true
}

func (f firefox) Install(der []byte) error {
	newDirs := missingDirs(f.dir())
	path, err := writeCert(f.dir(), der)
	if err != nil {
		return err
	}
	_, _, err = f.edit(func(in []string) []string {
		if slices.Contains(in, path) {
			return in
		}
		return append(in, path)
	}, newDirs)
	if err != nil {
		_, _ = removeCert(f.dir(), Thumbprint(der))
	}
	return err
}

func (f firefox) Remove(thumbprint string) error {
	path := certFile(f.dir(), thumbprint)
	deleted, dirs, err := f.edit(func(in []string) []string {
		return slices.DeleteFunc(in, func(p string) bool { return p == path })
	}, nil)
	if err != nil {
		return err
	}
	if _, err := removeCert(f.dir(), thumbprint); err != nil {
		return err
	}
	if deleted {
		for _, d := range dirs { // deepest first; a directory with anything else in it stays
			_ = os.Remove(d)
		}
	}
	return nil
}

func (f firefox) List(prefix string) ([]Cert, error) { return readCerts(f.dir(), prefix) }
