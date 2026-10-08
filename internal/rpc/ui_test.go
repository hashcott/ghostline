package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCallUI_GoesToCallingClient(t *testing.T) {
	_, sock := serve(t, ServiceHandler(&calc{}), acceptAll)
	a, b := dial(t, sock), dial(t, sock)
	var bAsked bool
	b.OnUI(func(context.Context, string, []json.RawMessage) (any, error) { bAsked = true; return "from b", nil })
	a.OnUI(func(_ context.Context, kind string, args []json.RawMessage) (any, error) {
		require.Equal(t, "pick", kind)
		require.JSONEq(t, `"q"`, string(args[0]))
		return "from a", nil
	})
	var got string
	require.NoError(t, a.Call(context.Background(), "Ask", &got))
	require.Equal(t, "from a", got)
	require.False(t, bAsked)
}

func TestCallUI_ClientGoneIsNoUI(t *testing.T) {
	var out string
	var callErr error
	done := make(chan struct{})
	h := func(ctx context.Context, method string, args []json.RawMessage) (json.RawMessage, error) {
		callErr = CallUI(ctx, "pick", &out)
		close(done)
		return nil, callErr
	}
	_, sock := serve(t, h, acceptAll)
	a := dial(t, sock)
	a.OnUI(func(context.Context, string, []json.RawMessage) (any, error) {
		_ = a.Close() // the GUI goes away while its dialog is open
		select {}
	})
	_ = a.Call(context.Background(), "Ask", nil)
	<-done
	require.ErrorIs(t, callErr, ErrNoUI)
}

func TestCallUI_OutsideCallIsNoUI(t *testing.T) {
	err := CallUI(context.Background(), "pick", nil)
	require.True(t, errors.Is(err, ErrNoUI))
	require.Equal(t, CodeNoUI, ErrNoUI.Error())
}
