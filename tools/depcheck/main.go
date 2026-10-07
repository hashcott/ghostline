// Command depcheck fails when a package pulls in a dependency it must not
// have, so OS-specific code cannot leak into another OS's binary.
//
//	go run ./tools/depcheck
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	failed := false
	for _, r := range Rules {
		deps, err := listDeps(r.GOOS, r.Pkg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "depcheck: %s (%s): %v\n", r.Pkg, r.GOOS, err)
			os.Exit(2)
		}
		for _, v := range Violations(deps, r.Forbid) {
			fmt.Printf("depcheck: %s for %s depends on %s: %s\n", r.Pkg, r.GOOS, v, r.Why)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
	fmt.Printf("depcheck: %d rules ok\n", len(Rules))
}

// listDeps lists pkg's transitive dependencies as a real build for goos
// sees them (cgo files included).
func listDeps(goos, pkg string) ([]string, error) {
	cmd := exec.Command("go", "list", "-e", "-deps", pkg)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=1")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// Violations returns the deps that match a forbid entry, in deps order.
func Violations(deps, forbid []string) []string {
	var out []string
	for _, d := range deps {
		for _, f := range forbid {
			if d == f || strings.HasPrefix(d, f+"/") {
				out = append(out, d)
				break
			}
		}
	}
	return out
}
