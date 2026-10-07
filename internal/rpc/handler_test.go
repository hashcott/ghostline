package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type calc struct{}

func (calc) Add(a, b int) int { return a + b }
func (calc) Div(a, b int) (int, error) {
	if b == 0 {
		return 0, errors.New("divide by zero")
	}
	return a / b, nil
}
func (calc) Pair() (int, string) { return 1, "x" }
func (calc) WithCtx(ctx context.Context, s string) string {
	if ctx == nil {
		return "nil ctx"
	}
	return s + "!"
}
func (calc) Ask(ctx context.Context) (string, error) {
	var out string
	err := CallUI(ctx, "pick", &out, "q")
	return out, err
}

func call(t *testing.T, h Handler, method string, args ...any) (json.RawMessage, error) {
	t.Helper()
	raw, err := encodeArgs(args)
	require.NoError(t, err)
	return h(context.Background(), method, raw)
}

func TestServiceHandler_DecodesArgsAndResults(t *testing.T) {
	h := ServiceHandler(&calc{})
	r, err := call(t, h, "Add", 1, 2)
	require.NoError(t, err)
	require.JSONEq(t, `3`, string(r))
	r, err = call(t, h, "Pair")
	require.NoError(t, err)
	require.JSONEq(t, `[1,"x"]`, string(r))
	r, err = call(t, h, "WithCtx", "hi")
	require.NoError(t, err)
	require.JSONEq(t, `"hi!"`, string(r))
	r, err = call(t, h, "Div", 6, 3)
	require.NoError(t, err)
	require.JSONEq(t, `2`, string(r))
}

func TestServiceHandler_ErrorAndUnknownMethod(t *testing.T) {
	h := ServiceHandler(&calc{})
	_, err := call(t, h, "Div", 1, 0)
	require.EqualError(t, err, "divide by zero")
	_, err = call(t, h, "Nope")
	require.EqualError(t, err, "rpc: unknown method Nope")
	_, err = call(t, h, "Add", "not a number", 2)
	require.Error(t, err)
	_, err = call(t, h, "Add", 1)
	require.Error(t, err, "wrong argument count")
}
