//go:build linux

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseArgs(t *testing.T) {
	cases := []struct {
		argv []string
		want Args
	}{
		{[]string{"--daemon"}, Args{Command: "daemon", AllowUID: -1}},
		{[]string{"--restore"}, Args{Command: "restore", AllowUID: -1}},
		{[]string{"--remove-certs"}, Args{Command: "remove-certs", AllowUID: -1}},
		{[]string{"--export", "/tmp/x.json"}, Args{Command: "export", ExportPath: "/tmp/x.json", AllowUID: -1}},
		{[]string{"status"}, Args{Command: "status", AllowUID: -1}},
		{[]string{"connect", "--socket", "/tmp/s"}, Args{Command: "connect", Socket: "/tmp/s", AllowUID: -1}},
		{[]string{"disconnect"}, Args{Command: "disconnect", AllowUID: -1}},
		{[]string{"--daemon", "--data-dir", "/tmp/d", "--allow-uid", "1000"}, Args{Command: "daemon", DataDir: "/tmp/d", AllowUID: 1000}},
	}
	for _, c := range cases {
		got, err := parseArgs(c.argv)
		require.NoError(t, err, c.argv)
		require.Equal(t, c.want, got, c.argv)
	}
	for _, bad := range [][]string{nil, {"--export"}, {"--bogus"}, {"--daemon", "status"}, {"--allow-uid", "x", "--daemon"}} {
		_, err := parseArgs(bad)
		require.Error(t, err, bad)
	}
}
