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
