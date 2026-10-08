package app

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The UI picks its wording by the platform the backend reports.
func TestSnapshot_PlatformFromDeps(t *testing.T) {
	h := newHarness(t)
	d := h.o.d
	d.Platform = "linux"
	require.Equal(t, "linux", New(d).Snapshot().Platform)
}
