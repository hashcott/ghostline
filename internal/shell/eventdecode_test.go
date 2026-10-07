package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/stretchr/testify/require"
)

// Review Focus 5: every typed event registered with Wails is decoded into
// exactly its registered type before the Linux GUI re-emits it.
func TestDecodeEvent_CoversEveryRegisteredEvent(t *testing.T) {
	names := map[string]string{
		"EventState": app.EventState, "EventStats": app.EventStats, "EventLog": app.EventLog, "EventQuery": app.EventQuery,
		"EventScan": app.EventScan, "EventAutotune": app.EventAutotune, "EventUpdate": app.EventUpdate,
		"EventProxyStats": app.EventProxyStats, "EventProxyConn": app.EventProxyConn, "EventRulesCompiled": app.EventRulesCompiled,
		"EventListsProgress": app.EventListsProgress, "EventDNSServerStats": app.EventDNSServerStats,
		"EventCertsChanged": app.EventCertsChanged, "EventSetupCountdown": app.EventSetupCountdown,
		"EventToolsScan": app.EventToolsScan, "EventToolsCFScan": app.EventToolsCFScan,
	}
	src, err := os.ReadFile("events.go")
	require.NoError(t, err)
	regs := regexp.MustCompile(`RegisterEvent\[(.+?)\]\(app\.(\w+)\)`).FindAllStringSubmatch(string(src), -1)
	require.Len(t, regs, 16)
	for _, r := range regs {
		name, ok := names[r[2]]
		require.True(t, ok, r[2])
		v, ok := decodeEvent(name, json.RawMessage("null"))
		require.True(t, ok, name)
		require.Equal(t, r[1], fmt.Sprintf("%T", v), name)
	}
	require.Len(t, eventDecoders, len(regs), "no extra names")
	_, ok := decodeEvent("settings", json.RawMessage("null"))
	require.False(t, ok, "settings is for the tray, not the frontend")
}
