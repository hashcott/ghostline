package app

import (
	"fmt"
	"testing"

	"github.com/hashcott/ghostline/internal/dpi"
	"github.com/stretchr/testify/require"
)

func TestDPIErr_KernelUnsupported(t *testing.T) {
	e := dpiErr(fmt.Errorf("prepare: %w", dpi.ErrKernelUnsupported), "zapret2")
	require.Equal(t, CodeDPIKernelUnsupported, e.Code)
}
