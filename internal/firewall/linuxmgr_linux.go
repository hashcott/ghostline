package firewall

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// backend opens single ports in one Linux firewall (firewalld, ufw, none).
type backend interface {
	exists(zone, proto string, port int) (bool, error)
	add(zone, proto string, port int, name string) error
	remove(zone, proto string, port int) error // already closed is not an error
	zone() (string, error)
	public() (bool, error)
}

// linuxRule is what a named rule opened, kept in a file because DeleteNamed
// only gets the name (and must work after a crash, from --restore).
type linuxRule struct {
	Zone  string `json:"zone"`
	Proto string `json:"proto"`
	Ports []int  `json:"ports"` // opened by Ghostline (not ones already open)
}

type linuxManager struct {
	path string
	b    backend
	mu   sync.Mutex
}

func newLinux(path string, b backend) Manager { return &linuxManager{path: path, b: b} }

func (m *linuxManager) load() (map[string]linuxRule, error) {
	rules := map[string]linuxRule{}
	b, err := os.ReadFile(m.path)
	if errors.Is(err, fs.ErrNotExist) {
		return rules, nil
	}
	if err != nil {
		return nil, err
	}
	return rules, json.Unmarshal(b, &rules)
}

func (m *linuxManager) save(rules map[string]linuxRule) error {
	if len(rules) == 0 {
		if err := os.Remove(m.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func (m *linuxManager) Add(port int) error {
	return m.AddNamed(Rule{Name: ProxyRule, Protocol: "TCP", Ports: []int{port}})
}

func (m *linuxManager) Delete() error { return m.DeleteNamed(ProxyRule) }

// AddNamed records what it will open before opening it, and leaves ports
// that were already open alone. Block rules have no Linux equivalent
// (firewalld's zones already decide what a public network lets in).
func (m *linuxManager) AddNamed(r Rule) error {
	if r.Block {
		return nil
	}
	if err := m.DeleteNamed(r.Name); err != nil {
		return err
	}
	zone, err := m.b.zone()
	if err != nil {
		return err
	}
	rule := linuxRule{Zone: zone, Proto: strings.ToLower(r.Protocol)}
	for _, p := range r.Ports {
		open, err := m.b.exists(zone, rule.Proto, p)
		if err != nil {
			return err
		}
		if !open {
			rule.Ports = append(rule.Ports, p)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rules, err := m.load()
	if err != nil {
		return err
	}
	rules[r.Name] = rule
	if err := m.save(rules); err != nil {
		return err
	}
	for _, p := range rule.Ports {
		if err := m.b.add(zone, rule.Proto, p, r.Name); err != nil {
			return err
		}
	}
	return nil
}

func (m *linuxManager) DeleteNamed(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rules, err := m.load()
	if err != nil {
		return err
	}
	rule, ok := rules[name]
	if !ok {
		return nil
	}
	var errs []error
	for _, p := range rule.Ports {
		errs = append(errs, m.b.remove(rule.Zone, rule.Proto, p))
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	delete(rules, name)
	return m.save(rules)
}

func (m *linuxManager) IsPublicNetwork() (bool, error) { return m.b.public() }

// none is a machine without a firewall Ghostline manages: nothing to open.
// public reports a drop-by-default input chain of another tool, which
// keeps LAN devices out all the same.
type none struct{ inputDrop func() (bool, error) }

func (none) exists(string, string, int) (bool, error) { return true, nil }
func (none) add(string, string, int, string) error    { return nil }
func (none) remove(string, string, int) error         { return nil }
func (none) zone() (string, error)                    { return "", nil }
func (n none) public() (bool, error)                  { return n.inputDrop() }
