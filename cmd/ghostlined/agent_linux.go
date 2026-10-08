package main

import (
	"io"
	"maps"

	"github.com/hashcott/ghostline/internal/session"
	"github.com/hashcott/ghostline/internal/sysproxy/desktop"
)

// agentTasks and agentStreams are everything --session-agent may run in a
// user's session; the daemon cannot ask for anything else.
func agentTasks() map[string]session.Task {
	t := map[string]session.Task{}
	maps.Copy(t, desktop.AgentTasks())
	return t
}

func agentStreams() map[string]session.StreamTask {
	s := map[string]session.StreamTask{}
	maps.Copy(s, desktop.AgentStreams())
	return s
}

// runSessionAgent serves one request from the daemon (--session-agent).
func runSessionAgent(in io.Reader, out io.Writer) int {
	return session.Serve(in, out, agentTasks(), agentStreams())
}
