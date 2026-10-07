package shell

import (
	"encoding/json"

	"github.com/hashcott/ghostline/internal/app"
	"github.com/hashcott/ghostline/internal/certstore"
	"github.com/hashcott/ghostline/internal/engine"
	"github.com/hashcott/ghostline/internal/proxy"
)

func dec[T any](data json.RawMessage) (any, error) {
	var v T
	err := json.Unmarshal(data, &v)
	return v, err
}

// eventDecoders turns each typed event from the daemon back into the type
// events.go registers with Wails (one entry per registration; a test
// keeps them in step).
var eventDecoders = map[string]func(json.RawMessage) (any, error){
	app.EventState:          dec[app.Snapshot],
	app.EventStats:          dec[app.StatsEvent],
	app.EventLog:            dec[app.LogEvent],
	app.EventQuery:          dec[engine.QueryEvent],
	app.EventScan:           dec[app.ScanProgress],
	app.EventAutotune:       dec[app.AutotuneProgress],
	app.EventUpdate:         dec[app.UpdateInfo],
	app.EventProxyStats:     dec[proxy.Stats],
	app.EventProxyConn:      dec[proxy.ConnEvent],
	app.EventRulesCompiled:  dec[app.RulesCompiled],
	app.EventListsProgress:  dec[app.ListsProgress],
	app.EventDNSServerStats: dec[engine.ServeStats],
	app.EventCertsChanged:   dec[[]certstore.Cert],
	app.EventSetupCountdown: dec[app.SetupCountdown],
	app.EventToolsScan:      dec[app.AdvScanProgress],
	app.EventToolsCFScan:    dec[app.CFProgress],
}

// decodeEvent decodes a daemon event for the frontend; false for names the
// frontend does not get (and for undecodable data).
func decodeEvent(name string, data json.RawMessage) (any, bool) {
	d, ok := eventDecoders[name]
	if !ok {
		return nil, false
	}
	v, err := d(data)
	return v, err == nil
}
