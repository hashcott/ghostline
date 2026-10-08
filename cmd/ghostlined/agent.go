package main

import (
	"io"

	"github.com/hashcott/ghostline/internal/session"
)

// agentTasks and agentStreams are everything --session-agent may run in a
// user's session; the daemon cannot ask for anything else.
func agentTasks() map[string]session.Task         { return map[string]session.Task{} }
func agentStreams() map[string]session.StreamTask { return map[string]session.StreamTask{} }

// runSessionAgent serves one request from the daemon (--session-agent).
func runSessionAgent(in io.Reader, out io.Writer) int {
	return session.Serve(in, out, agentTasks(), agentStreams())
}
