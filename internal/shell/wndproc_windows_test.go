package shell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassify(t *testing.T) {
	require.Equal(t, wmEndSession, classify(0x11, 1))
	require.Equal(t, wmEndSession, classify(0x11, 0))
	require.Equal(t, wmEndSession, classify(0x16, 1))
	require.Equal(t, wmNone, classify(0x16, 0))
	require.Equal(t, wmResume, classify(0x218, 0x12))
	require.Equal(t, wmNone, classify(0x218, 0x4))
	require.Equal(t, wmNone, classify(0x10, 0))
}
