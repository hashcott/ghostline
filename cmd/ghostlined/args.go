//go:build linux

package main

import (
	"errors"
	"fmt"
	"strconv"
)

// Args is ghostlined's command line.
type Args struct {
	Command    string // daemon | restore | remove-certs | export | install-system | uninstall-system | status | connect | disconnect
	Socket     string // --socket; "" = the platform's
	DataDir    string // --data-dir: development/test layout (platform.NewDev)
	AllowUID   int    // --allow-uid; -1 when unset (development/tests)
	ExportPath string
	Purge      bool // --purge with --uninstall-system: also the data and logs
}

const usage = "usage: ghostlined --daemon | --restore | --remove-certs | --export <file> | --install-system | --uninstall-system [--purge] | --session-agent | status | connect | disconnect [--socket <path>] [--data-dir <dir>] [--allow-uid <uid>]"

func parseArgs(argv []string) (Args, error) {
	a := Args{AllowUID: -1}
	set := func(cmd string) error {
		if a.Command != "" {
			return fmt.Errorf("two commands: %s and %s", a.Command, cmd)
		}
		a.Command = cmd
		return nil
	}
	value := func(i *int, flag string) (string, error) {
		if *i+1 >= len(argv) {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		*i++
		return argv[*i], nil
	}
	for i := 0; i < len(argv); i++ {
		var err error
		switch arg := argv[i]; arg {
		case "--daemon":
			err = set("daemon")
		case "--restore":
			err = set("restore")
		case "--remove-certs":
			err = set("remove-certs")
		case "--session-agent":
			err = set("session-agent")
		case "--install-system":
			err = set("install-system")
		case "--uninstall-system":
			err = set("uninstall-system")
		case "--purge":
			a.Purge = true
		case "--export":
			if err = set("export"); err == nil {
				a.ExportPath, err = value(&i, arg)
			}
		case "status", "connect", "disconnect":
			err = set(arg)
		case "--socket":
			a.Socket, err = value(&i, arg)
		case "--data-dir":
			a.DataDir, err = value(&i, arg)
		case "--allow-uid":
			var v string
			if v, err = value(&i, arg); err == nil {
				a.AllowUID, err = strconv.Atoi(v)
			}
		default:
			err = fmt.Errorf("unknown argument %q", arg)
		}
		if err != nil {
			return Args{}, err
		}
	}
	if a.Purge && a.Command != "uninstall-system" {
		return Args{}, errors.New("--purge needs --uninstall-system")
	}
	if a.Command == "" {
		return Args{}, errors.New("no command")
	}
	return a, nil
}
