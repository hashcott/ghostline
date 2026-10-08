package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViolations(t *testing.T) {
	deps := []string{"github.com/hashcott/ghostline/internal/model", "github.com/wailsapp/wails/v3/pkg/application"}
	require.Equal(t, []string{"github.com/wailsapp/wails/v3/pkg/application"}, Violations(deps, []string{"github.com/wailsapp/wails"}))
	require.Empty(t, Violations(deps, []string{"github.com/google/nftables"}))
}

func TestViolations_PrefixIsPathAware(t *testing.T) {
	require.Empty(t, Violations([]string{"github.com/hashcott/ghostline/internal/rpcx"}, []string{"github.com/hashcott/ghostline/internal/rpc"}))
}

// A mistyped Pkg must fail the check, not pass it with nothing checked.
func TestParseList_ErrorFailsTheCheck(t *testing.T) {
	deps, err := parseList("github.com/hashcott/ghostline/internal/model\ngithub.com/hashcott/ghostline/internal/app\n")
	require.NoError(t, err)
	require.Equal(t, []string{"github.com/hashcott/ghostline/internal/model", "github.com/hashcott/ghostline/internal/app"}, deps)

	_, err = parseList("github.com/hashcott/ghostline/cmd/ghostlined\tdirectory not found\n")
	require.ErrorContains(t, err, "directory not found")
}

// The Windows exe must not link the Linux session agent or nftables.
func TestRules_LinuxOnlyPackagesForbiddenOnWindows(t *testing.T) {
	var forbid []string
	for _, r := range Rules {
		if r.GOOS == "windows" && r.Pkg == "." {
			forbid = append(forbid, r.Forbid...)
		}
	}
	require.Contains(t, forbid, "github.com/hashcott/ghostline/internal/session")
	require.Contains(t, forbid, "github.com/google/nftables")
}
