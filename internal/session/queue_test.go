package session

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueue_DrainOnlyThatUIDAndKeepsFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-queue.json")
	q := NewQueue(path)
	require.NoError(t, q.Add(1000, "proxy.restore", map[string]string{"a": "1"}))
	require.NoError(t, q.Add(1001, "nss.remove", "x"))
	require.NoError(t, q.Add(1000, "nss.remove", "bad"))

	var ran []string
	err := NewQueue(path).Drain(User{UID: 1000}, func(task string, args json.RawMessage) error {
		ran = append(ran, task+":"+string(args))
		if string(args) == `"bad"` {
			return errors.New("still failing")
		}
		return nil
	})
	require.Error(t, err)
	require.Equal(t, []string{`proxy.restore:{"a":"1"}`, `nss.remove:"bad"`}, ran)

	var left []string
	require.NoError(t, NewQueue(path).Drain(User{UID: 1000}, func(task string, _ json.RawMessage) error { left = append(left, task); return nil }))
	require.Equal(t, []string{"nss.remove"}, left, "the failed item stayed")
	var other []string
	require.NoError(t, q.Drain(User{UID: 1001}, func(task string, _ json.RawMessage) error { other = append(other, task); return nil }))
	require.Equal(t, []string{"nss.remove"}, other)
}
