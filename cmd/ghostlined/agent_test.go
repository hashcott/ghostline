package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// --session-agent only ever runs the fixed task list.
func TestSessionAgent_RefusesUnknownTasks(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 2, runSessionAgent(bytes.NewBufferString(`{"task":"sh"}`+"\n"), &out))
	require.Contains(t, out.String(), "UNKNOWN_TASK")
}
