//go:build linux

// Command ghostlined is Ghostline's Linux daemon. It runs as root (a
// systemd service), owns DNS, DPI and the proxy, and serves the GUI over
// the control socket; it also restores and exports without a GUI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/headless"
	"github.com/hashcott/ghostline/internal/platform"
	"github.com/hashcott/ghostline/internal/rpc"
	"github.com/hashcott/ghostline/internal/sysinstall"
)

func main() {
	a, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghostlined:", err)
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	switch a.Command {
	case "session-agent":
		// Run as a user by the daemon: no lock, no log, no root setup.
		os.Exit(runSessionAgent(os.Stdin, os.Stdout))
	case "daemon":
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		defer stop()
		if err := runDaemon(ctx, a); err != nil {
			fmt.Fprintln(os.Stderr, "ghostlined:", err)
			os.Exit(1)
		}
	case "install-system", "uninstall-system":
		os.Exit(runSystemInstall(a))
	case "restore", "remove-certs", "export":
		p, err := depsFor(a)
		if err != nil {
			fmt.Fprintln(os.Stderr, "ghostlined:", err)
			os.Exit(1)
		}
		kind := map[string]cli.Kind{"restore": cli.KindRestore, "remove-certs": cli.KindRemoveCerts, "export": cli.KindExport}[a.Command]
		os.Exit(headless.Run(cli.Mode{Kind: kind, ExportPath: a.ExportPath}, p))
	default:
		os.Exit(runCLI(a))
	}
}

// runCLI talks to the running daemon: status, connect, disconnect.
func runCLI(a Args) int {
	socket := a.Socket
	if socket == "" {
		socket = platform.ClientSocket()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := rpc.Dial(ctx, socket, brand.Version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghostlined:", err)
		return 1
	}
	defer c.Close()
	switch a.Command {
	case "status":
		var sn struct {
			Status string `json:"status"`
			Error  *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		var raw json.RawMessage
		if err := c.Call(ctx, "GetSnapshot", &raw); err != nil {
			fmt.Fprintln(os.Stderr, "ghostlined:", err)
			return 1
		}
		_ = json.Unmarshal(raw, &sn)
		fmt.Println("status:", sn.Status)
		if sn.Error != nil {
			fmt.Println("error:", sn.Error.Code)
		}
		return 0
	case "connect":
		err = c.Call(ctx, "Connect", nil)
	case "disconnect":
		err = c.Call(ctx, "Disconnect", nil)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghostlined:", err)
		return 1
	}
	return 0
}

// runSystemInstall installs or removes the background service for the
// AppImage and the tar.gz (packages do this in their scripts).
func runSystemInstall(a Args) int {
	if os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "ghostlined: --%s needs root (sudo)\n", a.Command)
		return 1
	}
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghostlined:", err)
		return 1
	}
	in := sysinstall.Installer{Root: "/", Self: self, Sys: sysinstall.NewSystem()}
	if a.Command == "install-system" {
		err = in.Install()
	} else {
		err = in.Uninstall(a.Purge, func() error {
			// What a package's prerm runs: DNS, proxy, certificates,
			// firewall and the nftables table back as before.
			p, err := depsFor(a)
			if err != nil {
				return err
			}
			if headless.Run(cli.Mode{Kind: cli.KindRemoveCerts}, p) != 0 {
				return errors.New("restoring the system failed (see above)")
			}
			return nil
		})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ghostlined:", err)
		return 1
	}
	return 0
}
