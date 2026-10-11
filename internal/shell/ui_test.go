package shell

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A tray double-click delivers a second click while WebView2 is still being
// created for the first; the guard must drop it, not open a second window.
func TestReentryGuard_DropsNestedCall(t *testing.T) {
	var g reentryGuard
	created := 0
	open := func() {
		g.run(func() {
			created++
			g.run(func() { created++ }) // the click that arrives mid-creation
		})
	}
	open()
	require.Equal(t, 1, created)
	open() // free again once the first call returned
	require.Equal(t, 2, created)
}

func TestReentryGuard_ReleasesAfterPanic(t *testing.T) {
	var g reentryGuard
	require.Panics(t, func() { g.run(func() { panic("boom") }) })
	ran := false
	g.run(func() { ran = true })
	require.True(t, ran)
}
