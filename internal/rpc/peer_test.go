package rpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllowed(t *testing.T) {
	cases := []struct {
		name   string
		uid    uint32
		groups []string
		extra  []int
		want   bool
	}{
		{"root", 0, nil, nil, true},
		{"admin group", 1000, []string{"users", "wheel"}, nil, true},
		{"plain user", 1000, []string{"users"}, nil, false},
		{"extra uid", 1000, []string{"users"}, []int{1000}, true},
		{"nothing", 1000, nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, allowed(c.uid, c.groups, DefaultGroups, c.extra))
		})
	}
}
