package firewall

import (
	"fmt"
	"strings"
)

// runner runs a firewall tool.
type runner func(name string, args ...string) ([]byte, error)

// ufw opens ports with `ufw allow`, commented so they can be told apart.
type ufw struct{ run runner }

func ufwPort(port int, proto string) string { return fmt.Sprintf("%d/%s", port, proto) }

// exists reports a rule for exactly port/proto among the user's rules.
func (u ufw) exists(_, proto string, port int) (bool, error) {
	out, err := u.run("ufw", "show", "added")
	if err != nil {
		return false, err
	}
	want := "ufw allow " + ufwPort(port, proto)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == want || strings.HasPrefix(line, want+" ") {
			return true, nil
		}
	}
	return false, nil
}

func (u ufw) add(_, proto string, port int, name string) error {
	_, err := u.run("ufw", "allow", ufwPort(port, proto), "comment", "ghostline: "+name)
	return err
}

func (u ufw) remove(_, proto string, port int) error {
	out, err := u.run("ufw", "delete", "allow", ufwPort(port, proto))
	if err != nil && strings.Contains(string(out), "non-existent") {
		return nil
	}
	return err
}

func (ufw) zone() (string, error) { return "", nil }
func (ufw) public() (bool, error) { return false, nil }

// ufwActive reports `ufw status` saying the firewall is on.
func ufwActive(run runner) bool {
	out, err := run("ufw", "status")
	return err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), "Status: active")
}
