package dpi

import (
	"testing"

	"github.com/google/nftables/expr"
	"github.com/stretchr/testify/require"
)

// Root run: nft showed "ct packets 1-20" — the direction was not sent, so
// the kernel counted both directions. The queue rules must carry it.
func TestNftRule_QueueCountsOneDirection(t *testing.T) {
	for dir, want := range map[string]uint32{"original": ctDirOriginal, "reply": ctDirReply} {
		ex, err := nftRule{Chain: "post", Action: "queue", Proto: "tcp", Port: 443, Dir: dir, Packets: 20}.exprs()
		require.NoError(t, err)
		var ct *expr.Ct
		for _, e := range ex {
			if c, ok := e.(*expr.Ct); ok && c.Key == expr.CtKeyPKTS {
				ct = c
			}
		}
		require.NotNil(t, ct)
		require.True(t, ct.OptDirection, dir)
		require.Equal(t, want, ct.Direction, dir)
	}
}
