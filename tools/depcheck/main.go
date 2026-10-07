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
// sees them (cgo files included). Each line is "path" or "path<TAB>error".
func listDeps(goos, pkg string) ([]string, error) {
	cmd := exec.Command("go", "list", "-e", "-deps", "-f", "{{.ImportPath}}{{if .Error}}\t{{.Error.Err}}{{end}}", pkg)
	cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=1")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parseList(string(out))
}

// parseList reads listDeps' output. A package go list could not load (a
// mistyped Pkg, a missing directory) is an error: otherwise the rule would
// pass with nothing checked.
func parseList(out string) ([]string, error) {
	var deps []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		path, msg, bad := strings.Cut(line, "\t")
		if bad {
			return nil, fmt.Errorf("%s: %s", path, msg)
		}
		if path != "" {
			deps = append(deps, path)
		}
	}
	return deps, nil
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
