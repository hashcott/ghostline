package shell

import (
	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/proxy"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The UI's typed events. They live here, not in app, so app does not need
// Wails (the Linux daemon builds without it).
func init() {
	application.RegisterEvent[app.Snapshot](app.EventState)
	application.RegisterEvent[app.StatsEvent](app.EventStats)
	application.RegisterEvent[app.LogEvent](app.EventLog)
	application.RegisterEvent[engine.QueryEvent](app.EventQuery)
	application.RegisterEvent[app.ScanProgress](app.EventScan)
	application.RegisterEvent[app.AutotuneProgress](app.EventAutotune)
	application.RegisterEvent[app.UpdateInfo](app.EventUpdate)
	application.RegisterEvent[proxy.Stats](app.EventProxyStats)
	application.RegisterEvent[proxy.ConnEvent](app.EventProxyConn)
	application.RegisterEvent[app.RulesCompiled](app.EventRulesCompiled)
	application.RegisterEvent[app.ListsProgress](app.EventListsProgress)
	application.RegisterEvent[engine.ServeStats](app.EventDNSServerStats)
	application.RegisterEvent[[]certstore.Cert](app.EventCertsChanged)
	application.RegisterEvent[app.SetupCountdown](app.EventSetupCountdown)
	application.RegisterEvent[app.AdvScanProgress](app.EventToolsScan)
	application.RegisterEvent[app.CFProgress](app.EventToolsCFScan)
}
