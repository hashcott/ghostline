package watchdog_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/firewall"
	"github.com/hashcott/ghostline/internal/watchdog"
	"github.com/stretchr/testify/require"
)

// Recovery must try every rule Ghostline can create, or one survives a crash.
func TestWatchdogCleansEveryRule(t *testing.T) {
	require.Equal(t, firewall.AllRuleNames, watchdog.AllFirewallRules)
}
