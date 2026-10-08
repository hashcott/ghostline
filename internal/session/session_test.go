package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The test binary doubles as the agent: run with GHOSTLINE_TEST_AGENT=1 it
// serves these tasks on stdin/stdout instead of running tests.
var testTasks = map[string]Task{
	"echo": func(args json.RawMessage) (any, error) { return args, nil },
	"fail": func(json.RawMessage) (any, error) { return nil, &Error{Code: "BOOM", Message: "it failed"} },
	"env":  func(json.RawMessage) (any, error) { return os.Environ(), nil },
	"flood": func(json.RawMessage) (any, error) {
		return strings.Repeat("x", 2*maxReply), nil
	},
}

var testStreams = map[string]StreamTask{
	"tick": func(ctx context.Context, _ json.RawMessage, emit func(any)) error {
		for i := 0; ; i++ {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(20 * time.Millisecond):
				emit(i)
			}
		}
	},
}

func TestMain(m *testing.M) {
	if os.Getenv("GHOSTLINE_TEST_AGENT") == "1" {
		os.Exit(Serve(os.Stdin, os.Stdout, testTasks, testStreams))
	}
	os.Exit(m.Run())
}

func TestServe_RunsKnownTask(t *testing.T) {
	var out bytes.Buffer
	code := Serve(bytes.NewBufferString(`{"task":"echo","args":{"x":1}}`+"\n"), &out, testTasks, testStreams)
	require.Equal(t, 0, code)
	require.JSONEq(t, `{"result":{"x":1}}`, out.String())
}

func TestServe_UnknownTaskRefused(t *testing.T) {
	var out bytes.Buffer
	code := Serve(bytes.NewBufferString(`{"task":"rm -rf"}`+"\n"), &out, testTasks, testStreams)
	require.Equal(t, 2, code)
	require.JSONEq(t, `{"error":{"code":"UNKNOWN_TASK","message":"rm -rf"}}`, out.String())
}

func TestServe_TaskErrorKeepsItsCode(t *testing.T) {
	var out bytes.Buffer
	require.Equal(t, 1, Serve(bytes.NewBufferString(`{"task":"fail"}`+"\n"), &out, testTasks, testStreams))
	require.JSONEq(t, `{"error":{"code":"BOOM","message":"it failed"}}`, out.String())
}

func TestError_IsErrAgent(t *testing.T) {
	var err error = &Error{Code: "X"}
	require.ErrorIs(t, err, ErrAgent)
	var e *Error
	require.True(t, errors.As(err, &e))
	require.Equal(t, "X", e.Code)
}
