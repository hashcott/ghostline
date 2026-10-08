package sysinstall

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// NewSystem is System on this machine: systemctl and groupadd.
func NewSystem() System { return execSystem{} }

type execSystem struct{}

func run(name string, args ...string) error {
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (execSystem) Systemctl(args ...string) error { return run("systemctl", args...) }

func (execSystem) IsActive(unit string) bool {
	return exec.Command("systemctl", "is-active", "--quiet", unit).Run() == nil
}

func (execSystem) GroupExists(name string) bool {
	_, err := user.LookupGroup(name)
	return err == nil
}

func (execSystem) AddGroup(name string) error { return run("groupadd", "--system", name) }
