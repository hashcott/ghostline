package firewall

import (
	"fmt"
	"strconv"
	"strings"
)

// Netsh manages rules with Windows' netsh. Run and Public do the OS calls
// (see NewNetsh); everything else here is plain logic.
type Netsh struct {
	Exe    string                              // only this program gets the rules
	Run    func(args []string) ([]byte, error) // runs netsh
	Public func() (bool, error)                // network category check
}

// AddArgs are the netsh arguments for the LAN-sharing rule: only the
// proxy port, only Ghostline's exe, only Private networks, only the local
// subnet.
func AddArgs(port int, exe string) []string {
	return []string{"advfirewall", "firewall", "add", "rule", "name=" + ProxyRule, "dir=in", "action=allow",
		"protocol=TCP", "localport=" + strconv.Itoa(port), "program=" + exe, "profile=private", "remoteip=localsubnet"}
}

// DeleteArgs removes the LAN-sharing rule.
func DeleteArgs() []string {
	return []string{"advfirewall", "firewall", "delete", "rule", "name=" + ProxyRule}
}

func showArgs(name string) []string {
	return []string{"advfirewall", "firewall", "show", "rule", "name=" + name}
}

// RuleArgs are the netsh arguments for r: only Ghostline's exe, only
// Private networks, only the local subnet.
func RuleArgs(r Rule, exe string) []string {
	if r.Block {
		return []string{"advfirewall", "firewall", "add", "rule", "name=" + r.Name, "dir=in", "action=block",
			"program=" + exe, "profile=public"}
	}
	ports := make([]string, len(r.Ports))
	for i, p := range r.Ports {
		ports[i] = strconv.Itoa(p)
	}
	return []string{"advfirewall", "firewall", "add", "rule", "name=" + r.Name, "dir=in", "action=allow",
		"protocol=" + r.Protocol, "localport=" + strings.Join(ports, ","), "program=" + exe, "profile=private", "remoteip=localsubnet"}
}

// AddNamed (re)creates r.
func (n *Netsh) AddNamed(r Rule) error {
	if err := n.DeleteNamed(r.Name); err != nil {
		return err
	}
	if out, err := n.Run(RuleArgs(r, n.Exe)); err != nil {
		return fmt.Errorf("firewall: add rule %q: %v: %s", r.Name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteNamed removes the rule called name; a missing rule is not an error.
// netsh's messages are localised, so "missing" is decided by a failing
// "show rule" rather than by parsing text.
func (n *Netsh) DeleteNamed(name string) error {
	out, err := n.Run([]string{"advfirewall", "firewall", "delete", "rule", "name=" + name})
	if err == nil {
		return nil
	}
	if _, serr := n.Run(showArgs(name)); serr != nil {
		return nil // no such rule
	}
	return fmt.Errorf("firewall: delete rule %q: %v: %s", name, err, strings.TrimSpace(string(out)))
}

// Add (re)creates the LAN-sharing rule.
func (n *Netsh) Add(port int) error {
	if err := n.Delete(); err != nil {
		return err
	}
	if out, err := n.Run(AddArgs(port, n.Exe)); err != nil {
		return fmt.Errorf("firewall: add rule: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Delete removes the LAN-sharing rule; a missing rule is not an error.
func (n *Netsh) Delete() error {
	out, err := n.Run(DeleteArgs())
	if err == nil {
		return nil
	}
	if _, serr := n.Run(showArgs(ProxyRule)); serr != nil {
		return nil // no such rule
	}
	return fmt.Errorf("firewall: delete rule: %v: %s", err, strings.TrimSpace(string(out)))
}

// IsPublicNetwork reports whether a connected network uses the Public
// firewall profile.
func (n *Netsh) IsPublicNetwork() (bool, error) { return n.Public() }

// parsePublic reports whether any network category line is "Public".
func parsePublic(out string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.EqualFold(strings.TrimSpace(l), "Public") {
			return true
		}
	}
	return false
}
