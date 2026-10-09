package shell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAllowTaskbarCreated(t *testing.T) {
	require.NoError(t, allowTaskbarCreated())
}
